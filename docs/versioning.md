# Versioning Notes

Sentinel Airlock uses semantic-style tags for releases:
- `vMAJOR.MINOR.PATCH`
- optional release-candidate suffixes, e.g. `v2.2.0-rc1`

## Build metadata

The CLI embeds:
- version
- commit
- build date

Check with:

```bash
./airlock --version
```

## Release artifacts

`make release-artifacts VERSION=<version>` produces:
- `dist/airlock-<version>-darwin-amd64`
- `dist/airlock-<version>-darwin-arm64`
- `dist/airlock-<version>-linux-amd64`
- `dist/airlock-<version>-linux-arm64`
- `dist/build_info.json`
