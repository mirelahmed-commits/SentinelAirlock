# Sentinel Airlock v2.2.0-rc1 — Release Notes

**Date:** 2026-03-08  
**Type:** Release Candidate  
**Build:** Go 1.22 · single binary · local-first · no SaaS required

---

## Summary

v2.2.0-rc1 is the first release candidate of Sentinel Airlock: a governance and observability layer for coding agents. It delivers the full governed-run lifecycle, a tamper-evident evidence artifact model, all terminal operator flows (inspect/replay/verify/review/export), a polished static HTML evidence report, a local web evidence viewer, and a remote worker foundation.

---

## What Ships

### Core Run Engine

A 22-step governed execution pipeline that:
- Copies the repo to an isolated workspace before execution
- Classifies the agent command by risk level and category
- Blocks high-risk commands before execution (in `deny-high-risk` mode)
- Watches for file events during execution and reverts policy violations in-place
- Writes the full artifact set after every run

### Evidence Artifact Model

Every run produces, under `.airlock/runs/<run_id>/`:

| File | Description |
|---|---|
| `events.jsonl` | Structured event log — policy decisions, file events, risk classifications |
| `session_events.jsonl` | Model/tool/message session trace |
| `run_manifest.json` | Full run metadata — adapter, sandbox, network, policy, risk/approval summary |
| `run_digest.json` | SHA-256 digest of artifact set (tamper-evident) |
| `run_digest.sig` | Optional ed25519 signature over the digest |
| `changes.patch` | Unified diff of workspace changes vs original repo |
| `report/index.html` | Static HTML evidence report (self-contained, no external dependencies) |
| `checkpoints/cp-0/` | Full workspace snapshot taken before agent execution |
| `review.json` | Review decision artifact — state, note, reviewer, timestamp |

### Static HTML Evidence Report

Per-run `report/index.html` — self-contained, no CDN or JavaScript framework, works offline:

- Run overview (ID, status, adapter, timestamp, duration, workspace, command)
- Governance summary (policy pack, approval mode, sandbox mode, network mode)
- Risk summary (overall level, command category, final decision, event counts)
- Blocked-command alert (prominent, only when a command was blocked pre-execution)
- Allowed changes list
- Denied/reverted changes table (path, risk, reason, revert status)
- Patch summary with file count, insertion/deletion counts, collapsible diff preview
- Evidence timeline (all events; POLICY_DENY and command-block rows highlighted)
- Review status (state, reviewer, timestamp, note)
- Verification (per-artifact SHA-256, signed/unsigned status)
- Artifacts list (all evidence files, linked and sized)
- Limitations note with link to SECURITY.md

### Policy and Governance

- `airlock.yaml` per-project config: workspace ignores, `deny_read` / `deny_write` / `allow_write` glob patterns, network mode, signing key, team name
- Five built-in policy packs: `balanced`, `strict`, `ci-safe`, `oss-maintainer`, `research`
- Three execution mode presets (`dev`, `team`, `ci`) with automatic sandbox/network/approval defaults
- Risk classifier: low / medium / high + category label
- Approval modes: `auto`, `prompt`, `deny-high-risk`

### Sandbox Modes

- `workspace` (default): repo copied to isolated directory; filesystem watcher records all events; original repo untouched during execution
- `container`: agent executes inside Docker/Colima/Podman (auto-detected); stronger isolation
- `off`: no isolation; direct execution for environments with external controls

### Terminal Operator Flows

| Command | What it does |
|---|---|
| `airlock inspect <id>` | Pretty-print artifacts: adapter, sandbox, risk, policy, paths |
| `airlock replay <id>` | Terminal event-timeline replay with `⛔` markers for deny/block events |
| `airlock verify <id>` | SHA-256 digest check + optional signature verification |
| `airlock review <id>` | Persist review decision (state, note, reviewer, timestamp) |
| `airlock export <id>` | Evidence bundle as zip or tar.gz |
| `airlock patch <id>` | Apply or inspect `changes.patch` |
| `airlock serve` | Local HTTP evidence viewer (runs list, run detail, compare, review) |

### Web Evidence Viewer (`airlock serve --read-only`)

- Run list with ID, status, policy pack, sandbox, risk level, review state, timestamp
- Run detail with timeline, patch review, rollback, review form, export
- `/files/<run_id>/...` route for serving static report and artifacts directly
- "Open full report" button links to the static HTML evidence report
- `--read-only` flag disables review writes
- No authentication — restrict port access at OS/network level

### Remote Worker

- `airlock worker start`: HTTP job server; accepts jobs, executes locally, serves artifact bundles
- `airlock submit`: POST job to remote worker, poll for completion, download artifacts
- `airlock fetch <id>`: unpack remote artifacts to local `.airlock/runs/<run_id>/`; full local parity — all `inspect`/`replay`/`verify`/`serve` commands work identically on fetched runs

### Signing (Optional)

Set `AIRLOCK_SIGNING_KEY` (ed25519 private key, hex-encoded) to sign the run digest. `airlock verify` checks the signature. `run_digest.sig` is recorded as a permanent artifact.

---

## Demo Flows

### 90-Second Demo

```bash
make build
bash samples/demo.sh
```

Proves: build → bootstrap → governed run → inspect → replay → review → verify → export → web viewer.

### Full Governance Boundary Demo (~5 min)

```bash
bash samples/demo-full.sh
```

| Scenario | Agent behavior | Airlock outcome |
|---|---|---|
| Safe run | Writes to `src/` only | Allowed; clean artifacts |
| Policy denial | Writes `.env` + deploy path | POLICY_DENY; files reverted; `events.jsonl` shows diff |
| Command block | `rm -rf src` | Blocked before execution; `Status: failed class=policy` |

HTML evidence report paths printed at end of script.

---

## Installation

### Build from source

```bash
git clone <repo-url>
cd sentinel-airlock
make build
./airlock --version
```

### Install to PATH

```bash
make install PREFIX=$HOME/bin
# or
scripts/install.sh "$HOME/bin"
```

### First-time project setup

```bash
./airlock bootstrap
```

Creates `.airlock/` and a starter `airlock.yaml`.

### Requirements

| Tool | Version | Required for |
|---|---|---|
| Go | 1.22+ | Build |
| make | any | Build |
| Git | any | Clone |
| Docker / Colima / Podman | any | `--sandbox container` only |

Container runtime is optional. The default workspace sandbox runs without it.

---

## Known Limitations

| Limitation | Detail |
|---|---|
| Workspace sandbox caveat | Agent process runs on host OS. Workspace directory boundary is best-effort, not OS-enforced. Container recommended for stronger isolation. |
| Network enforcement | `off` mode in workspace sandbox records policy intent but does not block syscalls. Enforced network isolation requires container mode. |
| Only captures through Airlock | Only captures workflows launched through `airlock run`. Not a system-wide agent monitor. |
| Remote worker auth | Shared bearer token only — no per-user IAM, token rotation, or scoped access. Run behind TLS. |
| Web viewer auth | No authentication on `airlock serve`. Restrict port at OS/network level. `--read-only` disables writes. |
| Local-first | No hosted control plane, no telemetry, no external data transmission (unless using remote worker). |

---

## Compatibility Notes

- Artifact schema is stable at this RC. All artifact files are JSON/JSONL/text — readable without Airlock.
- `latest` alias supported across all commands: `inspect`, `replay`, `review`, `verify`, `export`.
- The static HTML evidence report (`report/index.html`) has no external dependencies and renders offline in any modern browser.
- Fetched remote run artifacts are fully compatible with local commands.

---

## Further Reading

| Document | Contents |
|---|---|
| [`docs/REVIEWER_GUIDE.md`](docs/REVIEWER_GUIDE.md) | End-to-end walkthrough + what to look for in reports |
| [`docs/architecture.md`](docs/architecture.md) | Execution boundary diagram, run lifecycle, artifact model |
| [`SECURITY.md`](SECURITY.md) | Sandbox model, guarantees, explicit limitations |
| [`samples/QUICKSTART.md`](samples/QUICKSTART.md) | Step-by-step copy-paste walkthrough |
| [`CHANGELOG.md`](CHANGELOG.md) | Detailed change log |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Build, test, extend |
