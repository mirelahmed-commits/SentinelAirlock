#!/usr/bin/env bash
# Sentinel Airlock — first reviewer demo (~90 seconds)
#
# Proves:
#   - agent command launched through Airlock
#   - workspace sandboxed from original repo
#   - full artifact set captured (events, manifest, digest, patch, report)
#   - tamper-evidence digest verifiable
#   - review gate persisted as artifact
#   - evidence bundle exportable
#
# Prerequisites: Go 1.22+, make
# No Docker. No network. No external API keys.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

echo "=========================================="
echo " Sentinel Airlock — Reviewer Demo"
echo "=========================================="
echo ""

# ── 1. Build ──────────────────────────────────
echo "[ 1/7 ] Building binary..."
make build --quiet
echo "        Binary: $(./airlock --version)"
echo ""

# ── 2. Bootstrap ──────────────────────────────
echo "[ 2/7 ] Bootstrapping project..."
./airlock bootstrap 2>&1 | grep -E "^(Bootstrap complete|Step|Policy path|OK|WARN)" || true
echo ""

# ── 3. Governed run ───────────────────────────
echo "[ 3/7 ] Running agent under Airlock governance..."
echo "        Command: mkdir -p src && echo 'Hello from Airlock' > src/hello.txt"
echo "        Sandbox: workspace (repo copy, original untouched)"
echo ""
./airlock run \
  --agent generic-shell \
  --cmd 'mkdir -p src && echo "Hello from Airlock demo run" > src/hello.txt && echo "Run at: $(date -u)" >> src/hello.txt' \
  --repo .
echo ""

# ── 4. Inspect ────────────────────────────────
echo "[ 4/7 ] Inspecting artifacts..."
./airlock inspect latest
echo ""

# ── 5. Replay ────────────────────────────────
echo "[ 5/7 ] Replaying event timeline..."
./airlock replay latest --tail 20
echo ""

# ── 6. Review + Verify ────────────────────────
echo "[ 6/7 ] Reviewing and verifying..."
./airlock review latest --state approved --note "Reviewer demo — clean safe run"
./airlock verify latest
echo ""

# ── 7. Export ────────────────────────────────
echo "[ 7/7 ] Exporting evidence bundle..."
./airlock export latest --format zip --include-report
echo ""

# ── Summary ──────────────────────────────────
RUN_DIR=".airlock/runs/$(ls -1t .airlock/runs | head -n 1)"
echo "=========================================="
echo " Demo complete. Artifacts:"
echo "   Manifest:  $RUN_DIR/run_manifest.json"
echo "   Events:    $RUN_DIR/events.jsonl"
echo "   Digest:    $RUN_DIR/run_digest.json"
echo "   Patch:     $RUN_DIR/changes.patch"
echo "   Report:    $RUN_DIR/report/index.html"
echo "   Review:    $RUN_DIR/review.json"
echo "   Export:    $(ls "$RUN_DIR"/airlock-run-*.zip 2>/dev/null || echo '<none>')"
echo ""
echo " To browse all runs:"
echo "   ./airlock serve --read-only"
echo "=========================================="

