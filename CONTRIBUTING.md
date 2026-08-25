# Contributing

## Development Setup

```bash
git clone <repo-url>
cd sentinel-airlock
make build
./airlock --version
```

Run the test suite:
```bash
go test ./...
```

Verify a clean build before submitting changes:
```bash
go test ./...
make build
./airlock bootstrap
```

## Project Layout

Key directories for contributors:

| Path | What it contains |
|---|---|
| `cmd/airlock/main.go` | Entry point — wires all Cobra subcommands |
| `internal/cli/` | One file per CLI command |
| `internal/adapters/` | Agent adapter layer — add new adapters here |
| `internal/policypack/` | Named policy packs — add new packs here |
| `internal/execution/` | Sandbox engine (workspace / container / off) |
| `internal/governance/` | Risk classification + approval decisions |
| `internal/policy/` | `airlock.yaml` config loader |
| `internal/runmeta/` | Run manifest, digest, artifact loader |
| `internal/web/` | Local HTTP evidence viewer |

## Adding a Policy Pack

Policy packs live in `internal/policypack/packs.go`. Each pack is a struct that overrides specific fields from the base `airlock.yaml` config. Packs are merged over the base config at run time.

Steps:
1. Add a new named entry to the pack registry in `packs.go`
2. Set only the fields the pack should override (deny patterns, network mode, approval mode, etc.)
3. Add the pack name to the `--policy-pack` flag description in `internal/cli/run.go`
4. Add a test case to verify the merged config is what you expect

## Adding an Adapter

Adapters live in `internal/adapters/`. Each adapter implements the `Adapter` interface:
- `Name() string`
- `Prepare(ctx RunContext) (Invocation, error)`

Steps:
1. Create a new file in `internal/adapters/` implementing the interface
2. Register the adapter by name in `internal/adapters/registry.go`
3. Add diagnostics support in `internal/agents/diagnostics.go` (version probe + install hints)
4. Document the adapter name in the `--agent` flag description in `internal/cli/run.go`

## Pull Request Expectations

- Keep changes focused and scoped to one concern
- Add or update tests for any logic changes
- Update `samples/QUICKSTART.md` if CLI behaviour changes
- Include before/after command output snippets for UX changes
- Do not change the artifact schema without updating `internal/runmeta/`

## Commit Hygiene

- Prefer small, reviewable commits
- Use clear commit messages: what changed and why
- Reference issues where relevant

## Reporting Issues

Open a GitHub issue with:
- Airlock version (`./airlock --version`)
- OS and Go version
- Minimal reproduction steps
- Actual vs expected output
