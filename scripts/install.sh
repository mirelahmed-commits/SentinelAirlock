#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TARGET="${1:-/usr/local/bin}"
VERSION="${VERSION:-dev}"
COMMIT="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo none)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

cd "$ROOT"

go build -ldflags "-X main.version=$VERSION -X main.commit=$COMMIT -X main.buildDate=$BUILD_DATE" -o airlock ./cmd/airlock

if [[ "$TARGET" == ~* ]]; then
  TARGET="${TARGET/#\~/$HOME}"
fi

mkdir -p "$TARGET"
cp airlock "$TARGET/airlock"

echo "Airlock installed"
echo "Binary: $TARGET/airlock"
echo "Version: ${VERSION} (commit=${COMMIT}, built=${BUILD_DATE})"
"$TARGET/airlock" --version || true
