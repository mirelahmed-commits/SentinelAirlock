# BYOM Agent Runtime Integration

> **Bring Your Own Model** — connect any process-based or LLM-backed agent to Sentinel Airlock governance.

This integration demonstrates how a custom local agent can be launched through Airlock, read project context, produce structured output, and be governed by Airlock policy — without requiring API keys, network access, or changes to the Airlock binary.

---

## What This Proves

When the BYOM agent runs under Airlock:

| Claim | Evidence |
|---|---|
| File writes are governed | `events.jsonl` — `FILE_CREATE`, `FILE_WRITE` with risk + approval on every write |
| Policy denials are enforced | `POLICY_DENY` events + file revert when agent writes to `.env` or `secrets/**` |
| Full audit trail | `run_manifest.json`, `changes.patch`, `run_digest.json` — tamper-evident |
| Digest verifiable | `airlock verify <run_id>` — returns `verified-unsigned` (or `verified-signed` if key configured) |
| HTML evidence report | `report/index.html` — self-contained, no external dependencies |

Airlock captures **process-level evidence**: commands, file events, risk classification, policy decisions. It does not capture model-internal chain-of-thought or token-level traces unless the adapter emits session events. The `generic-shell` adapter used here captures command output and file changes.

---

## Quick Start (Default Mode — No Dependencies)

```bash
# 1. Build Airlock (if not already built)
make build

# 2. Run the full BYOM demo
bash samples/demo-byom.sh
```

The demo runs two scenarios:
- **Scenario 1:** Agent reads project context, writes `docs/byom-agent-notes.md` (allowed)
- **Scenario 2:** Same, plus attempts to write `.env` and `secrets/demo.pem` (blocked by Airlock policy)

---

## Running Manually

```bash
# Normal governed run
./airlock run \
  --agent generic-shell \
  --cmd "python3 integrations/byom-agent/agent.py \
    --task 'Summarize project context' \
    --context README.md \
    --output docs/byom-agent-notes.md" \
  --repo samples/byom-workspace \
  --policy integrations/byom-agent/policy.airlock.yaml

# Governance test: attempt denied writes
./airlock run \
  --agent generic-shell \
  --cmd "python3 integrations/byom-agent/agent.py \
    --task 'Audit project with governance test' \
    --context README.md \
    --output docs/byom-agent-notes.md \
    --attempt-risky" \
  --repo samples/byom-workspace \
  --policy integrations/byom-agent/policy.airlock.yaml

# Inspect evidence
./airlock inspect latest
./airlock replay latest --tail 20
./airlock verify latest
open .airlock/runs/$(ls -1t .airlock/runs | head -1)/report/index.html
```

---

## Optional: Connect to a Local LLM

The `agent.py` includes a clearly marked `analyze_context()` function. Replace the deterministic logic there with a call to any LLM backend.

### Ollama

```bash
pip install ollama
```

```python
import ollama

def analyze_context(context: str, task: str) -> dict:
    response = ollama.chat(
        model="llama3",
        messages=[
            {"role": "system", "content": "You are a code analyst. Be concise."},
            {"role": "user", "content": f"Task: {task}\n\nContext:\n{context}"},
        ],
    )
    content = response["message"]["content"]
    return {
        "task": task,
        "context_lines": len(context.splitlines()),
        "context_words": len(context.split()),
        "context_code_blocks": 0,
        "headings": [],
        "bullet_count": 0,
        "summary": content,
        "backend": "ollama/llama3",
    }
```

### OpenAI-compatible endpoint (vLLM, llama.cpp, Ollama /v1)

```bash
pip install openai
```

```python
from openai import OpenAI

client = OpenAI(base_url="http://localhost:8080/v1", api_key="not-required")

def analyze_context(context: str, task: str) -> dict:
    response = client.chat.completions.create(
        model="local-model",
        messages=[
            {"role": "system", "content": "You are a code analyst. Be concise."},
            {"role": "user", "content": f"Task: {task}\n\nContext:\n{context}"},
        ],
        max_tokens=512,
    )
    content = response.choices[0].message.content
    return {
        "task": task,
        "context_lines": len(context.splitlines()),
        "context_words": len(context.split()),
        "context_code_blocks": 0,
        "headings": [],
        "bullet_count": 0,
        "summary": content,
        "backend": "openai-compatible",
    }
```

### LangChain / LangGraph

```bash
pip install langchain-core langchain-ollama
```

```python
from langchain_ollama import ChatOllama
from langchain_core.messages import HumanMessage, SystemMessage

llm = ChatOllama(model="llama3")

def analyze_context(context: str, task: str) -> dict:
    messages = [
        SystemMessage(content="You are a code analyst. Be concise."),
        HumanMessage(content=f"Task: {task}\n\nContext:\n{context}"),
    ]
    response = llm.invoke(messages)
    return {
        "task": task,
        "context_lines": len(context.splitlines()),
        "context_words": len(context.split()),
        "context_code_blocks": 0,
        "headings": [],
        "bullet_count": 0,
        "summary": response.content,
        "backend": "langchain-ollama/llama3",
    }
```

### Raw HTTP (no extra packages)

```python
import urllib.request
import json

def analyze_context(context: str, task: str) -> dict:
    payload = {
        "model": "llama3",
        "messages": [
            {"role": "user", "content": f"Task: {task}\n\nContext:\n{context}"}
        ],
        "stream": False,
    }
    req = urllib.request.Request(
        "http://localhost:11434/v1/chat/completions",
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        data = json.load(resp)
    content = data["choices"][0]["message"]["content"]
    return {
        "task": task,
        "context_lines": len(context.splitlines()),
        "context_words": len(context.split()),
        "context_code_blocks": 0,
        "headings": [],
        "bullet_count": 0,
        "summary": content,
        "backend": "raw-http/ollama",
    }
```

---

## Policy Configuration

`policy.airlock.yaml` in this directory governs what the agent can write:

| Path | Policy | Result |
|---|---|---|
| `docs/**` | `allow_write` | Agent output — allowed |
| `src/**`, `app/**` | `allow_write` | Code directories — allowed |
| `**/.env` | `deny_write` | Credential file — blocked + reverted |
| `secrets/**` | `deny_write` | Secret directory — blocked + reverted |

To use a stricter or looser policy:
```bash
./airlock run ... --policy-pack strict   # built-in strict pack
./airlock run ... --policy-pack oss-maintainer  # OSS-focused pack
./airlock run ... --policy path/to/custom.yaml  # custom policy file
```

---

## Governance Evidence After Each Run

```
.airlock/runs/<run_id>/
├── events.jsonl           ← FILE_CREATE, POLICY_DENY, CMD events
├── session_events.jsonl   ← tool calls (generic-shell: basic wrappers)
├── run_manifest.json      ← adapter, sandbox, policy, risk summary
├── run_digest.json        ← SHA-256 per artifact
├── changes.patch          ← workspace diff
├── report/index.html      ← static HTML evidence report
├── review.json            ← review decision artifact
└── checkpoints/cp-0/      ← pre-run workspace snapshot
```

```bash
# Review the evidence
airlock inspect <run_id>
airlock replay <run_id> --tail 20     # ⛔ on denied events
airlock verify <run_id>               # verified-unsigned or verified-signed
airlock export <run_id> --format zip --include-report
```

---

## Limitations

- Airlock captures **process-level evidence** for `generic-shell` agents: file events, command output, risk/policy decisions. It does not capture model-internal reasoning or token-level traces.
- Adapters that emit session events (model/tool/message) populate `session_events.jsonl`. The `generic-shell` adapter emits basic tool-call wrappers; a dedicated model adapter would emit richer session traces.
- Network `off` mode in workspace sandbox is advisory — policy intent is recorded but not kernel-enforced. Use `--sandbox container` for enforced network isolation.
- This integration uses `--sandbox workspace` (default). For stronger process isolation, use `--sandbox container`.
