## Summary
<!-- One or two sentences: what this PR does. -->

## Why
<!-- What problem it solves or what was broken. Link the issue if one exists: Closes #N -->

## What changed
<!-- Key files and logic changes. Be specific enough that a reviewer can follow without reading every line. -->

## Validation
<!-- What you ran. Copy-paste or describe actual output. -->

## Risk / compatibility
<!-- Does this change CLI flags, artifact schema, policy semantics, or adapter behavior? If none: "none". -->

## Docs updated?
<!-- Did you update README, LIMITATIONS.md, REVIEWER_GUIDE, or inline help text if user-facing behavior changed? -->

---

**Checklist**
- [ ] Change is focused on one concern (no unrelated cleanup bundled in)
- [ ] Tests added or updated if logic changed
- [ ] `gofmt -w ./...` run locally
- [ ] `go vet ./...` passes
- [ ] `go test ./...` passes
- [ ] `make build` passes
- [ ] No `.airlock/`, binaries, logs, secrets, or generated release artifacts committed
- [ ] Docs updated if user-facing behavior changed
