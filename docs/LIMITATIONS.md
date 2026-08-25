# Known Limitations and Non-Goals

## Current limitations

- Agent backends (Codex, Claude Code, etc.) must be installed separately.
- Container mode depends on host runtime/socket permissions.
- Remote mode uses shared-token auth today; full IAM is out of scope.
- Some constrained environments block loopback bind for `serve` or `worker`.
- Network allowlist guarantees are strongest in containerized paths.

## Non-goals (current stage)

- Full SaaS multi-tenant control plane.
- Full enterprise identity/SSO/RBAC system.
- Replacing endpoint security or network perimeter controls.
- Becoming a model provider or chat frontend.
