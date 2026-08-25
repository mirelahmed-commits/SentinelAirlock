# Release Checklist

## Pre-flight
- [ ] `go test ./...` passes
- [ ] `make build VERSION=<version>` passes
- [ ] `./airlock --version` shows version/commit/build date
- [ ] `./airlock doctor` passes with no BLOCKER lines
- [ ] 90-second demo path verified from clean state: `rm -rf .airlock && bash samples/demo.sh && ./airlock verify latest`
- [ ] Governance boundary demo verified: `bash samples/demo-full.sh && ./airlock verify latest`
- [ ] Rollback demo verified: `bash samples/demo-rollback.sh && ./airlock verify latest`
- [ ] All three demos end with `status=verified-unsigned`
- [ ] `./airlock export latest --format zip --include-report && ./airlock verify latest` returns `verified-unsigned`

## Artifacts
- [ ] `make release-artifacts VERSION=<version>`
- [ ] Dist files generated with expected names:
  - `dist/airlock-darwin-amd64`
  - `dist/airlock-darwin-arm64`
  - `dist/airlock-linux-amd64`
  - `dist/airlock-linux-arm64`

## Docs
- [ ] `README.md` updated
- [ ] `CHANGELOG.md` updated
- [ ] `samples/QUICKSTART.md` updated
- [ ] `docs/messaging-pack.md` reviewed

## Release notes
- [ ] Fill `.github/release-notes-template.md`
- [ ] Tag created (`vX.Y.Z`)
- [ ] Notes include known limitations and non-goals

## Post-release
- [ ] Demo assets updated in `docs/assets/`
- [ ] Feedback template prepared (`docs/early-user-feedback.md`)
