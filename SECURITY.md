# Security

## Scope

This document describes the security model, guarantees, and explicit limitations of Sentinel Airlock v2.2.0-rc1.

Airlock is an agent-governance boundary and observability layer for coding agent execution. **It is not a security boundary in the OS or hypervisor sense.** It only captures workflows launched through Airlock — it is not a system-wide agent monitor. Read this document before deploying with untrusted agents or commands.

---

## Sandbox Modes

Set via `--sandbox` flag or `defaults.sandbox` in `airlock.yaml`.

### `workspace` (default)

The repo is copied to an isolated directory (`.airlock/workspaces/<run_id>/repo`) before the agent executes. A filesystem watcher records all file events against that directory.

**What it provides:**
- Original working directory is not modified during execution
- Pre-run snapshot written to `checkpoints/cp-0/` for comparison
- All file reads, writes, and policy denials recorded in `events.jsonl`
- `changes.patch` diff generated against original repo state

**Workspace sandbox caveat:** The agent process runs on the host OS with the privileges of the current user. It can access anything outside the workspace directory that the current user can access. The workspace directory boundary is best-effort, not a hard OS-enforced isolation guarantee. Container mode is recommended for stronger isolation.

---

### `container`

The agent executes inside a Docker, Colima, or Podman container. Airlock auto-detects the available runtime. Provides stronger process and filesystem isolation than workspace mode.

**Limitation:** Container security depends on your runtime configuration. Airlock does not configure seccomp profiles, AppArmor/SELinux policies, rootless settings, or read-only filesystem mounts. If you need hardened container execution, configure those controls at the runtime level independently.

**Fallback:** If `--fallback-workspace` is set and no container runtime is found, Airlock falls back to workspace mode. The `run_manifest.json` always records which sandbox mode was actually used.

---

### `off`

No sandboxing. The agent executes directly in the working directory with no isolation.

**Use only in environments where isolation is handled externally.** Not recommended for untrusted agents or commands.

---

## Network Controls

| Mode | Behaviour |
|---|---|
| `off` (default) | No outbound network — recorded as policy intent |
| `on` | Unrestricted outbound network |
| `allowlist` | Only whitelisted hostnames are permitted |

**Limitation:** In workspace mode, network enforcement is advisory — policy intent is recorded and `POLICY_DENY` events fire, but no kernel-level syscall blocking is applied. For enforced network isolation, use container mode with appropriate runtime network configuration.

---

## What Airlock Guarantees

- **Audit trail completeness:** Every run produces a structured event log, session trace, manifest, and patch — all written before the run is marked complete.
- **Tamper evidence:** `run_digest.json` contains a SHA-256 hash of the core evidence artifacts: `events.jsonl`, `session_events.jsonl`, `run_manifest.json`, `changes.patch`, and a hash of checkpoint file names. `airlock verify` checks these hashes. Sanctioned mutations by Airlock commands (`export`, `rollback`) rebuild the digest automatically so `verify` continues to return `verified-unsigned` after those operations. Third-party modifications to any digested artifact will cause `hash-mismatch`.
- **Optional cryptographic signing:** If `AIRLOCK_SIGNING_KEY` (ed25519) is configured, Airlock signs the digest file. This allows verification that the artifact set has not changed since it was produced.
- **Policy recording:** All policy decisions — allow, deny, risk level, approval mode — are recorded in `events.jsonl` and `run_manifest.json`, regardless of whether a command was blocked.
- **Review chain:** `review.json` records reviewer state, note, reviewer identity (from `$USER`), and timestamp as a permanent run artifact.

---

## What Airlock Does Not Guarantee

- **Workspace mode does not prevent host file access.** The process boundary is the OS process, not the workspace directory.
- **Airlock does not validate the agent binary.** It trusts the adapter and the executable it resolves.
- **Digest verification proves artifact integrity post-run, not execution integrity.** It confirms recorded artifacts haven't been modified after the run ended; it does not cryptographically prove what happened during execution.
- **Network mode `off` in workspace mode is not kernel-enforced.** It records a policy intent but does not block syscalls.
- **No multi-tenant isolation.** All runs on a machine share `.airlock/` and run as the same OS user.
- **The web viewer (`airlock serve`) has no authentication.** Anyone with access to the machine and port can read all run artifacts. Use `--read-only` to disable review writes.
- **`airlock rollback` restores the execution workspace, not the original repo.** It restores `.airlock/workspaces/<run_id>/repo` (the isolated sandbox copy). The original `--repo` source directory is not modified by Airlock at any point. One checkpoint per run (`cp-0`). Operation-level rollback (undo last N agent operations) is future work.
- **BYOM/generic-shell agents: process-level evidence only.** The `generic-shell` adapter captures commands, file events, risk classification, and policy decisions. It does not capture model-internal reasoning, chain-of-thought, or token-level traces. If you need session-level traces (model messages, tool calls, model responses), use an adapter that emits session events to `session_events.jsonl`. The BYOM integration (`integrations/byom-agent/`) documents this explicitly.

---

## Remote Worker Security

The remote worker (`airlock worker start`) uses a shared bearer token for authentication.

**Current limitations at v2.2.0-rc1:**
- Single shared secret — no per-user identity, roles, or scopes
- No token rotation mechanism built in
- Anyone holding the token can submit jobs and retrieve any artifact
- The worker has no per-submitter audit log

**Operational guidance:**
- Always run the worker behind TLS (reverse proxy) in any non-local deployment
- Treat the auth token as a high-value secret — rotate by restarting the worker with a new token
- Do not expose the worker port to untrusted networks

Full IAM (per-user tokens, roles, scoped access) is planned for a future release.

---

## Local-First Status

Sentinel Airlock v2.2.0-rc1 is **entirely local**. There is no hosted control plane, no telemetry collection, and no data transmitted externally unless you explicitly configure a remote worker and use `airlock submit`.

---

## Reporting a Vulnerability

Please do not open a public GitHub issue for security vulnerabilities.

Include in your report:
- Impact summary
- Reproduction steps
- Affected versions
- Suggested mitigation (if known)

We aim to acknowledge reports within 48 hours and provide a remediation timeline within 7 days for confirmed issues.
