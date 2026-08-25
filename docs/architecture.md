# Sentinel Airlock — Architecture

## Execution Boundary Diagram

```
  ┌─────────────────────────────────────────────────────────┐
  │                    airlock run                          │
  │                                                         │
  │  ┌──────────┐   Policy + Risk   ┌──────────────────┐  │
  │  │  Config   │ ───────────────▶ │   Governance     │  │
  │  │ airlock   │                  │ ClassifyCommand() │  │
  │  │  .yaml    │                  │ Decide() → allow  │  │
  │  └──────────┘                   │           deny    │  │
  │                                 └────────┬─────────┘  │
  │                                          │ allow        │
  │                                          ▼              │
  │  ┌──────────┐   Copy repo     ┌──────────────────┐    │
  │  │  Repo    │ ──────────────▶ │   Workspace      │    │
  │  │ (source) │                 │ .airlock/work-   │    │
  │  └──────────┘                 │ spaces/<id>/repo │    │
  │                               └────────┬─────────┘    │
  │                                        │               │
  │  ┌──────────┐  Execute in sandbox      ▼               │
  │  │  Agent   │ ──────────────▶ ┌──────────────────┐    │
  │  │ Adapter  │                 │ Sandbox Engine    │    │
  │  │(generic- │                 │  workspace mode:  │    │
  │  │  shell,  │                 │  dir copy + watcher│   │
  │  │  codex…) │                 │  container mode:  │    │
  │  └──────────┘                 │  Docker/Colima/   │    │
  │                               │  Podman           │    │
  │                               └────────┬─────────┘    │
  │                                        │               │
  │         FS Watcher (recorder)          │               │
  │  FILE_CREATE / FILE_WRITE / POLICY_DENY│               │
  │  ◀─────────────────────────────────────               │
  │         ▼                                              │
  │  ┌──────────────────────────────────────────────────┐ │
  │  │                  Artifact Set                     │ │
  │  │  events.jsonl   session_events.jsonl              │ │
  │  │  run_manifest.json   changes.patch                │ │
  │  │  run_digest.json     run_digest.sig (optional)    │ │
  │  │  report/index.html   review.json                  │ │
  │  │  checkpoints/cp-0/                                │ │
  │  └──────────────────────────────────────────────────┘ │
  └─────────────────────────────────────────────────────────┘
```

---

## Run Lifecycle

Every `airlock run` follows this 22-step pipeline:

```
 1. Load airlock.yaml + merge policy pack (balanced/strict/ci-safe/…)
 2. Apply execution mode defaults (dev/team/ci → sandbox/network/approval)
 3. Resolve adapter by name (generic-shell, codex, ollama, …)
 4. Generate run_id (UUID); create .airlock/runs/<run_id>/
 5. Open events logger (events.jsonl) + session sink (session_events.jsonl)
 6. Emit RUN_START event
 7. Copy repo → isolated workspace (.airlock/workspaces/<run_id>/repo)
 8. Snapshot workspace as checkpoint cp-0
 9. Adapter.Prepare() → Invocation{Executable, Args, DisplayCommand}
10. Resolve sandbox mode (workspace / container / off)
    → container: DetectRuntime() → fallback to workspace if --fallback-workspace
11. Evaluate env denials, sensitive path references, network allowlist
12. governance.ClassifyCommand() + Decide() → allow or block
    → block: emit CMD event (risk=high, approval=deny), skip to step 16
13. recorder.New() + rec.Start() — watch workspace for file events
14. execution.Run() — executes agent in sandbox
15. rec.Stop() — flush recorded file events
16. gitops.CreatePatchForPaths() → changes.patch
17. Build RunManifest (policy, sandbox, risk/approval summary, touched/denied paths)
18. runmeta.BuildDigest() → run_digest.json (SHA-256 per artifact)
19. Optional: ed25519 sign digest → run_digest.sig
20. report.Generate() → report/index.html (static, self-contained, no external deps)
21. output.PrintRunSummary() → terminal summary + next-step hints
22. refreshIndex() → update .airlock/index.json
```

---

## Artifact Model

All artifacts for a run are stored under `.airlock/runs/<run_id>/`:

| File | Description |
|---|---|
| `events.jsonl` | Structured event log — policy decisions, file events, risk classifications |
| `session_events.jsonl` | Model/tool/message session trace (adapter-dependent) |
| `run_manifest.json` | Full run metadata — adapter, sandbox, network, policy, risk/approval summary |
| `run_digest.json` | SHA-256 digest of artifact set (tamper-evident) |
| `run_digest.sig` | Optional ed25519 signature over the digest |
| `changes.patch` | Unified diff — workspace state vs original repo at run start |
| `report/index.html` | Static HTML evidence report (self-contained, no external dependencies) |
| `checkpoints/cp-0/` | Full workspace snapshot taken before agent execution |
| `review.json` | Review decision artifact — state, note, reviewer, timestamp |
| `review_events.jsonl` | Audit log of review state changes |
| `rollback.json` | Rollback record — checkpoint, mode, paths, timestamp, status (written by `airlock rollback`) |
| `build_info.json` | Airlock version, commit, build date at run time |

The artifact set is designed for offline use. All post-run commands (`inspect`, `replay`, `verify`, `serve`) read from this set with no live agent connection.

---

## Policy Model

### Config hierarchy

```
airlock.yaml (per-project)
  + policy pack merge (--policy-pack balanced|strict|ci-safe|oss-maintainer|research)
  + execution mode defaults (--mode dev|team|ci)
  + per-run flags (--sandbox, --network, --approval)
```

### Path policy

```yaml
policy:
  deny_read:  ["**/.env", "**/*.pem", "**/.ssh/**"]
  deny_write: [".git/**", ".airlock/**"]
  allow_write: ["src/**", "app/**"]
```

`deny_write` and `deny_read` are evaluated against every file event by the filesystem watcher. A write matching `deny_write` is immediately reverted and a `POLICY_DENY` event is recorded with a diff.

### Risk classification

`governance.ClassifyCommand()` maps commands to:
- **Level:** `low` / `medium` / `high`
- **Category:** `command`, `file`, `network`, `secret`, etc.

High-risk patterns (e.g. `rm -rf`, `chmod -r`, `curl | sh`) are caught pre-execution.

### Approval modes

| Mode | Behavior |
|---|---|
| `auto` | Allow all (record everything) |
| `prompt` | Interactive — ask the operator at run time |
| `deny-high-risk` | Block any command classified `high` before execution |

### Execution mode presets

| Mode | Sandbox | Network | Approval |
|---|---|---|---|
| `dev` (default) | workspace | off | auto |
| `team` | container | allowlist | deny-high-risk |
| `ci` | container | off | deny-high-risk |

---

## Sandbox Modes

### `workspace` (default)

Repo is copied to an isolated directory before execution. A filesystem watcher records every file event. The original working directory is not modified during execution.

**Limitation:** The agent process runs on the host OS. The workspace directory boundary is best-effort, not OS-enforced. Container mode is recommended for stronger isolation.

### `container`

Agent executes inside a Docker, Colima, or Podman container. Airlock auto-detects the available runtime. Provides stronger process and filesystem isolation.

`--fallback-workspace`: falls back to workspace mode if no runtime is found. The actual mode used is always recorded in `run_manifest.json`.

**Limitation:** Container security depends on runtime configuration. Airlock does not configure seccomp, AppArmor, or rootless settings. Configure those at the runtime level.

### `off`

No isolation. Agent executes directly in the working directory. Use only when isolation is handled externally.

---

## Replay / Review / Verify Model

These three commands operate entirely on the artifact set — no agent required:

**`airlock replay <id>`**
- Reads `events.jsonl` + `session_events.jsonl`
- Merges and sorts by timestamp
- Prints terminal timeline with markers: `⛔` for denied/blocked events, `·` for file changes
- `--tail N` to show last N rows; `--json` for structured output

**`airlock review <id> --state <state>`**
- Writes `review.json`: `{ state, note, reviewer, timestamp }`
- Valid states: `unreviewed` / `approved` / `rejected` / `needs-attention`
- Appends a `REVIEW_UPDATED` event to `review_events.jsonl`
- Regenerates `report/index.html` so review state is reflected immediately

**`airlock verify <id>`**
- Reads `run_digest.json` (SHA-256 per artifact)
- Recomputes hashes for: `events.jsonl`, `session_events.jsonl`, `run_manifest.json`, `changes.patch`, `checkpoints.meta`
- Compares stored vs current
- If signed: verifies ed25519 signature against `run_digest.sig`
- Outputs: `verified-signed` / `verified-unsigned` / `hash-mismatch` / `signature-invalid`

**Digest policy:**

| Artifact | In digest | Notes |
|---|---|---|
| `events.jsonl` | ✓ | Core evidence; also mutated by `rollback` which rebuilds digest |
| `session_events.jsonl` | ✓ | Session trace |
| `run_manifest.json` | ✓ | Also mutated by `export` (adds export path); export rebuilds digest |
| `changes.patch` | ✓ | Workspace diff |
| `checkpoints.meta` | ✓ | Hash of checkpoint file names |
| `report/index.html` | ✗ | Display artifact; regenerated by `review`/`rollback` |
| `review.json` | ✗ | Mutable by design — review gate |
| `rollback.json` | ✗ | Written by `rollback` |
| `review_events.jsonl` | ✗ | Review audit log |
| `run_digest.json` | ✗ | Cannot hash itself |
| `run_digest.sig` | ✗ | Signature over digest |
| `build_info.json` | ✗ | Written by `export` |
| `airlock-run-*.zip` | ✗ | Export bundle |

Sanctioned mutations (by `export` and `rollback`) rebuild `run_digest.json` automatically so `airlock verify` continues to return `verified-unsigned` after those commands. Third-party modifications to any digested file will produce `hash-mismatch`.

---

## Rollback Model

**`airlock rollback <id>`** (or `latest`) restores the isolated run workspace from checkpoint `cp-0`.

### What it restores

The workspace at `.airlock/workspaces/<run_id>/repo` — the directory where the agent executed. The original source repo (`--repo` path) is never modified by Airlock at any point.

### Modes

| Mode | Command |
|---|---|
| Full restore | `airlock rollback <id>` |
| Subtree restore | `airlock rollback <id> --path src/slides` |
| Preview only | `airlock rollback <id> --dry-run` |
| Skip prompt | `airlock rollback <id> --force` |

### Post-rollback artifact updates

After restoring the workspace, Airlock updates the artifact set so it stays consistent:

1. Appends a `ROLLBACK` event to `events.jsonl` with mode, checkpoint, and paths
2. Sets `review.json` to `needs-attention` — a prior approval is not silently left in place
3. Rebuilds `run_digest.json` — so `airlock verify` returns `verified-unsigned`, not `hash-mismatch`
4. Writes `rollback.json` — permanent rollback record (run_id, checkpoint, mode, timestamp, status)
5. Regenerates `report/index.html` — rollback event and new review state appear immediately

### Limitations (v2.2.0-rc1)

- **Workspace-only.** Does not touch your original `--repo` path.
- **One checkpoint per run (`cp-0`)**, taken before agent execution starts.
- **No operation-level rollback** — cannot undo the last N agent moves. Future work.
- **No patch-reverse** — `changes.patch` has absolute paths into the workspace; applying in reverse is not supported in this release.

---

## Remote Worker Model

```
┌─────────────────────────────┐     ┌───────────────────────────┐
│         operator            │     │       remote worker        │
│                             │     │  airlock worker start      │
│  airlock submit             │────▶│  → runs job locally        │
│  (POST /jobs, poll)         │     │  → same pipeline as local  │
│                             │◀────│  → artifact bundle upload  │
│  airlock fetch <id>         │     └───────────────────────────┘
│  → unpacks to .airlock/runs/│
│                             │
│  airlock inspect/verify/… │
│  (identical to local runs) │
└─────────────────────────────┘
```

Fetched remote runs produce the same artifact set as local runs. All local commands (`inspect`, `replay`, `verify`, `serve`) work identically on fetched artifacts.

**Current limitation:** Shared bearer token auth only. No per-user IAM, roles, or token rotation at v2.2.0-rc1. Always run the worker behind TLS for non-local deployments.

---

## Package Layout

```
internal/
├── adapters/       Agent adapter layer — resolves named adapters
├── agents/         Diagnostics for installed agent backends
├── cli/            One file per CLI command
├── events/         Structured event logger → events.jsonl
├── execution/      Sandbox engine (workspace / container / off)
├── gitops/         Patch generation (workspace diff → changes.patch)
├── governance/     Risk classification + approval decisions
├── index/          Fast run listing (.airlock/index.json)
├── output/         CLI summary printer
├── policy/         airlock.yaml config loader
├── policypack/     Named policy packs (balanced/strict/ci-safe/…)
├── providers/      LLM provider clients (anthropic/openai/ollama)
├── recorder/       Filesystem watcher (FILE_READ/FILE_WRITE/POLICY_DENY)
├── remote/         Remote worker HTTP protocol
├── replay/         Terminal timeline replay engine
├── report/         Static HTML evidence report generator
├── review/         review.json artifact (state/note/reviewer/timestamp)
├── runmeta/        Run manifest, digest, artifact loader
├── runner/         Internal runner core
├── session/        Session event sink → session_events.jsonl
├── util/           Shared helpers
├── web/            Local HTTP evidence viewer
└── workspace/      Repo copy/isolation
```
