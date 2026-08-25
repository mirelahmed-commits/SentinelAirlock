#!/usr/bin/env bash
# Sentinel Airlock — UI rollback demo (operator mode, browser-driven)
#
# This demo does NOT run rollback for you. It proves the *browser* path:
#   pre-rollback page -> click "Restore workspace" -> confirm -> the page
#   visibly changes to "Workspace rolled back" (rollback events 0 -> 1,
#   review -> needs-attention, verify stays clean).
#
# What "rollback" means here:
#   Airlock restores .airlock/workspaces/<run_id>/repo — the isolated execution
#   sandbox. Your original source directory is NEVER modified. Workspace-only.
#
# Prerequisites: Go 1.22+, make. No Docker, no network, no API keys.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

DEMO_REPO="samples/rollback-workspace"
PORT="${PORT:-8099}"

echo "==========================================================="
echo " Sentinel Airlock — UI Rollback Demo (operator mode)"
echo "==========================================================="
echo ""

# ── 1. Build ──────────────────────────────────────────────────
echo "[ 1/6 ] Building binary..."
make build --quiet
echo "        Binary: $(./airlock --version)"
echo ""

# ── 2. Stop any existing viewer, start a fresh operator viewer ─
echo "[ 2/6 ] Starting operator viewer in the background..."
./airlock serve --stop >/dev/null 2>&1 || true
./airlock serve --background --port "$PORT" >/dev/null 2>&1 || true
sleep 1
./airlock serve --status
echo ""

# ── 3. Create a fresh PRE-rollback run (deterministic command) ─
# NOTE: `mkdir -p docs` first — the workspace has no docs/ dir, so writing
# docs/new-file.md without it would fail and make the run look broken.
echo "[ 3/6 ] Creating a fresh governed run (pre-rollback)..."
./airlock run \
  --repo "$DEMO_REPO" \
  --agent generic-shell \
  --sandbox workspace \
  --policy-pack strict \
  --cmd "bash -lc 'mkdir -p docs; echo changed >> src/restore.md; echo changed >> src/keep.md; echo new > docs/new-file.md'" \
  > /tmp/airlock-ui-rollback-run.log 2>&1 || true

RID="$(grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' /tmp/airlock-ui-rollback-run.log | head -1)"
if [ -z "$RID" ]; then
  echo "ERROR: could not determine run id. See /tmp/airlock-ui-rollback-run.log"
  ./airlock serve --stop >/dev/null 2>&1 || true
  exit 1
fi
RUN_URL="http://127.0.0.1:${PORT}/runs/${RID}"
echo "        Run ID:  $RID"
echo "        Run URL: $RUN_URL"
echo ""

# ── 4. Open the run and describe the before/after states ───────
echo "[ 4/6 ] Opening the run page in your browser..."
./airlock serve --status >/dev/null 2>&1 || true
if command -v open >/dev/null 2>&1; then open "$RUN_URL" >/dev/null 2>&1 || true
elif command -v xdg-open >/dev/null 2>&1; then xdg-open "$RUN_URL" >/dev/null 2>&1 || true; fi
echo ""
echo "  BEFORE CLICK (what you should see now):"
echo "    - Governance badge:   rollback: available  (+ 'rollback available')"
echo "    - Rollback section:   'Rollback available — restore workspace from cp-0'"
echo "    - A button:           'Restore workspace'"
echo "    - Replay summary:     rollback events = 0"
echo "    - NO 'Workspace rolled back' card"
echo ""
echo "  ACTION:"
echo "    Click 'Restore workspace', accept the confirmation dialog."
echo "    The button shows 'Restoring…' and the page returns with a green"
echo "    'Rollback complete — workspace restored from cp-0.' banner."
echo ""
echo "  AFTER CLICK (the page should now show):"
echo "    - Top card:           '↩ Workspace rolled back'"
echo "    - Governance badge:   rollback: complete  +  rolled back (full)"
echo "    - Replay summary:     rollback events = 1"
echo "    - review:             needs-attention"
echo "    - verify:             verified-unsigned (clean)"
echo ""

# ── 5. Wait for the operator to perform the click ──────────────
echo "[ 5/6 ] Perform the click in the browser, then press Enter here to verify..."
read -r _ || true
echo ""
echo "        Verifying run $RID from artifacts:"
if [ -f ".airlock/runs/$RID/rollback.json" ]; then
  echo "        rollback.json     present ✓"
else
  echo "        rollback.json     NOT found — did you click 'Restore workspace'?"
fi
if grep -q '"ROLLBACK"' ".airlock/runs/$RID/events.jsonl" 2>/dev/null || grep -q 'ROLLBACK' ".airlock/runs/$RID/events.jsonl" 2>/dev/null; then
  echo "        ROLLBACK event    recorded ✓"
fi
./airlock verify "$RID" || true
echo ""

# ── 6. Stop the viewer cleanly ─────────────────────────────────
echo "[ 6/6 ] Stopping the viewer..."
./airlock serve --stop || true
echo ""
echo "==========================================================="
echo " UI rollback demo complete."
echo ""
echo " This demo proved the browser click path — the script itself"
echo " never ran rollback. Read-only mode (serve --read-only) would"
echo " show 'Rollback instructions' with no mutating button instead."
echo "==========================================================="
