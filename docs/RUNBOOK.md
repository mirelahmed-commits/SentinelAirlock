# Sentinel Airlock — Runbook

Canonical, copy-paste command flow for the current Airlock CLI. Every command below exists in the
current codebase (`internal/cli/`) as of this runbook's last update — see [Keeping this in
sync](#keeping-this-in-sync) if you're changing commands.

For a narrated first-run walkthrough with sample output, see
[`samples/QUICKSTART.md`](../samples/QUICKSTART.md). This document is the terser, authoritative
reference — the one to follow when you just need the right command.

## 1. Install

```bash
# Option A — go install (Go 1.22+)
go install github.com/mirelahmed-commits/SentinelAirlock/cmd/airlock@latest

# Option B — curl installer (macOS/Linux, no Go required)
curl -fsSL https://raw.githubusercontent.com/mirelahmed-commits/SentinelAirlock/main/scripts/install.sh | bash

# Option C — build from source
git clone https://github.com/mirelahmed-commits/SentinelAirlock.git
cd SentinelAirlock
make build      # produces ./airlock
```

## 2. Check the environment

```bash
airlock doctor
```

Checks the local environment, runtimes, and directory writability. `airlock agents doctor` checks
readiness of individual agent backends (`generic-shell`, `codex`, `ollama`, ...).

## 3. Bootstrap a project

```bash
cd your-project
airlock bootstrap
```

Writes `airlock.yaml` (via `airlock init` if it doesn't exist yet) and the `.airlock/` directory,
then runs the environment/agent/config doctor checks. Prints next-step guidance when done.

## 4. Configure policy

```bash
airlock policy configure   # interactive: pick deny-path presets + network mode
airlock policy show        # print the effective policy (packs, network, allow/deny rules)
```

`policy configure` is interactive and requires a real terminal — it prompts for common
sensitive-path deny rules (`.env`, `*.key`, `*.pem`, `secrets/**`, `credentials/**`, custom paths)
and a network mode, then writes the result back into `airlock.yaml`, preserving everything else in
the file. It refuses to run (with a clear message, no hang) if stdin isn't a terminal — pipe input
or edit `airlock.yaml` directly instead.

Other policy subcommands:

```bash
airlock policy list           # list available policy packs
airlock policy apply <pack>   # write a named policy pack to airlock.yaml
```

## 5. Run an agent under governance

```bash
airlock run --agent generic-shell --cmd 'mkdir -p src && echo hi > src/test.txt' --repo .
```

Prints a `run_id`. Use the literal word `latest` in place of a run ID in any command below to mean
"the most recent run."

Key flags: `--agent`, `--cmd`, `--repo`, `--policy`, `--policy-pack`, `--mode` (dev|team|ci),
`--sandbox` (off|workspace|container), `--network` (off|on|allowlist), `--approval`
(auto|prompt|deny-high-risk). Run `airlock run --help` for the full list.

## 6. Inspect and verify evidence

```bash
airlock inspect latest
airlock verify latest
```

`inspect` pretty-prints the run's artifacts (manifest, events, risk/policy decisions). `verify`
checks the SHA-256 digest (and signature, if configured) for tamper evidence.

Related: `airlock replay latest` (terminal event-timeline replay), `airlock patch latest`
(inspect/apply `changes.patch`), `airlock compare <run_a> <run_b>` (diff two runs).

## 7. Review

```bash
airlock review latest --state approved --note "looks clean"
```

`--state` is one of `unreviewed|approved|rejected|needs-attention`. Persists to `review.json`.

## 8. Rollback (if needed)

```bash
airlock rollback latest                  # restore full workspace from checkpoint
airlock rollback latest --dry-run        # preview without modifying anything
airlock rollback latest --path src/foo   # restore only a subtree
```

Restores the **Airlock workspace** (`.airlock/workspaces/<id>/repo`) from its checkpoint — never
your original repo.

## 9. Export

```bash
airlock export latest --format zip --include-report
```

`--format` is `zip` or `tar.gz`. `--include-report` (default `true`) bundles the self-contained
HTML report.

## 10. Browse runs in the viewer

```bash
airlock serve --open                          # foreground, operator mode, opens browser
airlock serve --read-only --open              # foreground, read-only (safe to share)
airlock serve --background --open             # detached, operator mode
airlock serve --background --read-only --open # detached, read-only
airlock serve --status                        # mode / URL / PID / log
airlock serve --stop                          # clean shutdown
```

Operator mode allows UI-driven review/rollback/export (with confirmation). Read-only mode returns
`403` on all mutating endpoints — safe to hand to someone else. `--background` detachment is not
supported on Windows; run `airlock serve` in a separate terminal there instead.

Default port is `8080`. If it's already in use, pass `--port` with any free port instead:

```bash
airlock serve --open --port 8082
```

## Full flow, start to finish

```bash
airlock doctor
airlock bootstrap
airlock policy configure
airlock policy show
airlock run --agent generic-shell --cmd 'mkdir -p src && echo hi > src/test.txt' --repo .
airlock inspect latest
airlock verify latest
airlock review latest --state approved --note "looks clean"
airlock export latest --format zip --include-report
airlock serve --open
airlock serve --status
airlock serve --stop
```

## Other commands

| Command | Purpose |
|---|---|
| `airlock cleanup` | Prune old run artifacts and stale workspaces |
| `airlock agents list` / `agents doctor` | List/diagnose agent backends |
| `airlock config get` / `config resolve` / `config set` / `config doctor` | Inspect/edit config defaults |
| `airlock worker start` | Start a remote worker server |
| `airlock submit` | Submit a run to a local or remote target |
| `airlock fetch <id>` | Pull remote run artifacts into local `.airlock/runs/` |
| `airlock index rebuild` / `index stats` | Rebuild or inspect the local run index |
| `airlock whoami` | Print caller identity context |

## Keeping this in sync

This file lists real commands as of the commit that introduced it. If you add, rename, or remove a
CLI command in `internal/cli/`, update this runbook (and the README Quick Start / CLI Reference
table) in the same change.
