# byom-workspace

This is the demo project workspace for the **Sentinel Airlock BYOM Agent Runtime Integration**.

When you run the BYOM demo, Airlock copies this directory to an isolated workspace, then the BYOM agent reads this `README.md` as its project context and writes a structured analysis to `docs/byom-agent-notes.md`.

---

## Project Overview

**byom-workspace** is a minimal example project with:

- Source code in `src/`
- Documentation in `docs/`
- Configuration files at the root

The BYOM agent is configured to:
- Read project context from `README.md`
- Write its output to `docs/byom-agent-notes.md`
- Respect the Airlock policy defined in `integrations/byom-agent/policy.airlock.yaml`

---

## Structure

```
byom-workspace/
├── README.md               ← project context (this file)
├── src/
│   └── processor.py        ← sample source file
├── docs/
│   └── .gitkeep            ← placeholder; agent writes here
└── .env.example            ← example config (not a real secret)
```

---

## Dependencies

This workspace has no dependencies. It exists purely to give the BYOM agent a meaningful context to read and analyze.

```
python3 -c "print('no setup required')"
```

---

## Governance

This workspace is governed by Airlock during agent runs:

- `docs/**` — agent can write here (structured notes, summaries)
- `src/**` — agent can write here (code-level changes)
- `**/.env` — blocked (no credential writes)
- `secrets/**` — blocked (no secret-directory writes)

All writes are recorded in `events.jsonl` and verifiable with `airlock verify`.

---

## Source Reference

- BYOM integration: `integrations/byom-agent/`
- Demo script: `samples/demo-byom.sh`
- Policy file: `integrations/byom-agent/policy.airlock.yaml`
