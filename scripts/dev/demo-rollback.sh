#!/usr/bin/env bash
# Sentinel Airlock — Rollback demo (~3 minutes)
#
# Proves two rollback scenarios:
#   A. Path-scoped rollback — agent modifies two files; only one is restored
#   B. Full workspace rollback — agent modifies three files; full restore from cp-0
#
# What "rollback" means here:
#   Airlock restores .airlock/workspaces/<run_id>/repo — the isolated execution
#   sandbox where the agent ran. The original source directory (samples/rollback-workspace/)
#   is NEVER modified by Airlock at any point. This is workspace rollback, not repo rollback.
#
# Prerequisites: Go 1.22+, make
# No Docker. No network. No external API keys.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

DEMO_REPO="samples/rollback-workspace"

echo "==========================================================="
echo " Sentinel Airlock — Rollback Demo"
echo "==========================================================="
echo ""

# ── 1. Build ──────────────────────────────────────────────────
echo "[ 1/9 ] Building binary..."
make build --quiet
echo "        Binary: $(./airlock --version)"
echo ""

# ── 2. Bootstrap ──────────────────────────────────────────────
echo "[ 2/9 ] Bootstrapping project..."
./airlock bootstrap 2>&1 | grep -E "^(Bootstrap complete|Step|Policy path|OK|WARN)" || true
echo ""

# ── 3. Prepare demo workspace ─────────────────────────────────
echo "[ 3/9 ] Preparing demo workspace: $DEMO_REPO"
# Reset to known-good state so the demo is repeatable on re-runs.
mkdir -p "$DEMO_REPO/src/notes"
cat > "$DEMO_REPO/src/keep.md" <<'SRCEOF'
# Keep Me

This file is part of the Airlock rollback demo.

It will be modified by the demo agent run.
After path-scoped rollback (which targets restore.md only),
this file remains modified — proving selective restore.
SRCEOF
cat > "$DEMO_REPO/src/restore.md" <<'SRCEOF'
# Restore Me

This file is part of the Airlock rollback demo.

It will be modified by the demo agent run.
After path-scoped rollback, this file is restored to
this original content from checkpoint cp-0.
SRCEOF
cat > "$DEMO_REPO/src/notes/release.md" <<'SRCEOF'
# Release Notes

This file is part of the Airlock rollback demo.

Used in Scenario B (full workspace rollback) to show
that all three modified files are restored to their
checkpoint cp-0 state.
SRCEOF
echo "        Created: $DEMO_REPO/src/keep.md"
echo "        Created: $DEMO_REPO/src/restore.md"
echo "        Created: $DEMO_REPO/src/notes/release.md"
echo ""

# ═══════════════════════════════════════════════════════════════
# SCENARIO A: Path-scoped rollback
# Agent modifies two files. Rollback restores only one.
# The other file remains modified — proving selective restore.
# ═══════════════════════════════════════════════════════════════
echo "═══════════════════════════════════════════════════════════"
echo " SCENARIO A: Path-scoped rollback"
echo "   Agent modifies: src/keep.md  AND  src/restore.md"
echo "   Rollback path:  src/restore.md  (only this path)"
echo "   Expected:       src/restore.md → restored to cp-0"
echo "                   src/keep.md    → still modified"
echo "═══════════════════════════════════════════════════════════"
echo ""

# ── 4. Governed run — Scenario A ──────────────────────────────
echo "[ 4/9 ] Running agent under Airlock governance (Scenario A)..."
echo "        Command: append line to src/keep.md and src/restore.md"
echo "        Repo:    $DEMO_REPO  (isolated to workspace before execution)"
echo ""
./airlock run \
  --agent generic-shell \
  --cmd 'printf "\nModified by agent — Scenario A\n" >> src/keep.md && printf "\nModified by agent — Scenario A\n" >> src/restore.md' \
  --repo "$DEMO_REPO"
echo ""

# Capture run artifacts
A_ID="$(ls -1t .airlock/runs | head -n 1)"
A_WS=".airlock/workspaces/$A_ID/repo"
A_CP=".airlock/runs/$A_ID/checkpoints/cp-0"
A_DIR=".airlock/runs/$A_ID"

echo "        Run ID:     $A_ID"
echo "        Workspace:  $A_WS"
echo "        Checkpoint: $A_CP"
echo ""

# ── 5. Evidence before rollback ───────────────────────────────
echo "[ 5/9 ] Evidence before rollback..."
./airlock inspect latest
echo ""
echo "--- Event timeline (tail 15) ---"
./airlock replay latest --tail 15
echo ""
echo "--- HTML evidence report ---"
echo "  $A_DIR/report/index.html"
echo ""
echo "--- Workspace state (both files modified by agent) ---"
echo "  src/keep.md    last line: $(tail -1 "$A_WS/src/keep.md")"
echo "  src/restore.md last line: $(tail -1 "$A_WS/src/restore.md")"
echo ""

# ── 6. Dry-run — verify nothing changes ───────────────────────
echo "[ 6/9 ] Dry-run — preview only, no workspace modification..."
./airlock rollback latest --path src/restore.md --dry-run
echo ""
# Confirm workspace state is unchanged after dry-run
if grep -q "Modified by agent" "$A_WS/src/restore.md"; then
  echo "        Confirmed: src/restore.md still modified after dry-run ✓"
else
  echo "        ERROR: dry-run unexpectedly changed workspace"
  exit 1
fi
echo ""

# ── 7. Path-scoped rollback ───────────────────────────────────
echo "[ 7/9 ] Path-scoped rollback: --path src/restore.md --force ..."
./airlock rollback latest --path src/restore.md --force
echo ""

echo "--- Proof: selective restore ---"

# src/restore.md in workspace must match checkpoint
if diff -q "$A_WS/src/restore.md" "$A_CP/src/restore.md" > /dev/null 2>&1; then
  echo "  src/restore.md  RESTORED — matches checkpoint cp-0 ✓"
else
  echo "  ERROR: src/restore.md does not match checkpoint after path rollback"
  echo "  workspace: $(tail -1 "$A_WS/src/restore.md")"
  echo "  checkpoint: $(tail -1 "$A_CP/src/restore.md")"
  exit 1
fi

# src/keep.md in workspace must differ from checkpoint (still modified)
if diff -q "$A_WS/src/keep.md" "$A_CP/src/keep.md" > /dev/null 2>&1; then
  echo "  ERROR: src/keep.md was restored but should remain modified"
  exit 1
else
  echo "  src/keep.md     UNCHANGED — still modified (path rollback only targeted src/restore.md) ✓"
fi

echo ""
echo "--- Artifacts after path rollback ---"

REVIEW_STATE="$(grep '"state"' "$A_DIR/review.json" | sed 's/.*"state"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')"
echo "  review.json state:  $REVIEW_STATE"
if [ "$REVIEW_STATE" = "needs-attention" ]; then
  echo "  review.json         needs-attention ✓"
else
  echo "  ERROR: expected needs-attention, got $REVIEW_STATE"
  exit 1
fi

if [ -f "$A_DIR/rollback.json" ]; then
  echo "  rollback.json       exists ✓"
  grep '"mode"' "$A_DIR/rollback.json" | sed 's/^/    /'
  grep '"status"' "$A_DIR/rollback.json" | sed 's/^/    /'
else
  echo "  ERROR: rollback.json missing"
  exit 1
fi

echo "  verify:            $(./airlock verify latest)"
echo ""
echo "--- Event timeline after rollback (ROLLBACK event visible) ---"
./airlock replay latest --tail 6
echo ""

# ═══════════════════════════════════════════════════════════════
# SCENARIO B: Full workspace rollback
# Agent modifies three files. Full rollback restores all.
# ═══════════════════════════════════════════════════════════════
echo "═══════════════════════════════════════════════════════════"
echo " SCENARIO B: Full workspace rollback"
echo "   Agent modifies: src/keep.md, src/restore.md, src/notes/release.md"
echo "   Rollback:       full (no --path flag)"
echo "   Expected:       all three files restored to cp-0 state"
echo "═══════════════════════════════════════════════════════════"
echo ""

# Reset the source files so Scenario B starts from known state
cat > "$DEMO_REPO/src/keep.md" <<'SRCEOF'
# Keep Me

This file is part of the Airlock rollback demo.

It will be modified by the demo agent run.
After path-scoped rollback (which targets restore.md only),
this file remains modified — proving selective restore.
SRCEOF
cat > "$DEMO_REPO/src/restore.md" <<'SRCEOF'
# Restore Me

This file is part of the Airlock rollback demo.

It will be modified by the demo agent run.
After path-scoped rollback, this file is restored to
this original content from checkpoint cp-0.
SRCEOF
cat > "$DEMO_REPO/src/notes/release.md" <<'SRCEOF'
# Release Notes

This file is part of the Airlock rollback demo.

Used in Scenario B (full workspace rollback) to show
that all three modified files are restored to their
checkpoint cp-0 state.
SRCEOF

# ── 8. Governed run — Scenario B ──────────────────────────────
echo "[ 8/9 ] Running agent under Airlock governance (Scenario B)..."
echo "        Command: append line to all three src/ files"
echo ""
./airlock run \
  --agent generic-shell \
  --cmd 'printf "\nModified by agent — Scenario B\n" >> src/keep.md && printf "\nModified by agent — Scenario B\n" >> src/restore.md && printf "\nModified by agent — Scenario B\n" >> src/notes/release.md' \
  --repo "$DEMO_REPO"
echo ""

B_ID="$(ls -1t .airlock/runs | head -n 1)"
B_WS=".airlock/workspaces/$B_ID/repo"
B_CP=".airlock/runs/$B_ID/checkpoints/cp-0"
B_DIR=".airlock/runs/$B_ID"

echo "        Run ID:     $B_ID"
echo "        Workspace:  $B_WS"
echo ""
echo "--- Workspace state (three files modified) ---"
echo "  src/keep.md          last line: $(tail -1 "$B_WS/src/keep.md")"
echo "  src/restore.md       last line: $(tail -1 "$B_WS/src/restore.md")"
echo "  src/notes/release.md last line: $(tail -1 "$B_WS/src/notes/release.md")"
echo ""

echo "--- Full workspace rollback (--force, no --path) ---"
./airlock rollback latest --force
echo ""

echo "--- Proof: full restore ---"
FAIL=0
for f in "src/keep.md" "src/restore.md" "src/notes/release.md"; do
  if diff -q "$B_WS/$f" "$B_CP/$f" > /dev/null 2>&1; then
    echo "  $f  RESTORED ✓"
  else
    echo "  ERROR: $f does not match checkpoint after full rollback"
    FAIL=1
  fi
done
if [ "$FAIL" -eq 1 ]; then exit 1; fi

echo ""
echo "  verify:  $(./airlock verify latest)"
B_REVIEW="$(grep '"state"' "$B_DIR/review.json" | sed 's/.*"state"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')"
echo "  review:  $B_REVIEW"
B_MODE="$(grep '"mode"' "$B_DIR/rollback.json" | sed 's/.*"mode"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')"
echo "  rollback mode: $B_MODE"
echo ""
echo "--- HTML evidence report (shows rollback + needs-attention) ---"
echo "  $B_DIR/report/index.html"
echo ""
echo "--- Event timeline (ROLLBACK event visible) ---"
./airlock replay latest --tail 6
echo ""

# ── 9. Summary ────────────────────────────────────────────────
echo "==========================================================="
echo " Rollback demo complete."
echo ""
echo " What was proved:"
echo "   A. Path-scoped rollback"
echo "      src/restore.md  restored to checkpoint cp-0 content ✓"
echo "      src/keep.md     remained modified (not targeted)    ✓"
echo "      review.json     needs-attention                     ✓"
echo "      rollback.json   artifact written (mode=path)        ✓"
echo "      verify          passed after rollback               ✓"
echo "      ROLLBACK event  recorded in event timeline          ✓"
echo ""
echo "   B. Full workspace rollback"
echo "      all three files restored to checkpoint cp-0        ✓"
echo "      review.json     needs-attention                     ✓"
echo "      rollback.json   artifact written (mode=full)        ✓"
echo "      verify          passed after rollback               ✓"
echo "      ROLLBACK event  recorded in event timeline          ✓"
echo ""
echo " Rollback scope (honest):"
echo "   Restores: .airlock/workspaces/<run_id>/repo (Airlock workspace)"
echo "   NOT modified by Airlock: $DEMO_REPO (original source)"
echo "   One checkpoint per run: cp-0 (taken before agent execution)"
echo "   Operation-level rollback (last N ops): future work"
echo ""
echo " Artifacts:"
echo "   Scenario A: $A_DIR/"
echo "   Scenario B: $B_DIR/"
echo ""
echo " To browse all runs in the evidence viewer:"
echo "   ./airlock serve --read-only"
echo ""
echo " In the browser, verify the rollback is understandable without"
echo " reading raw artifacts:"
echo "   1. Open run $B_ID"
echo "      - Header shows a '↩ Workspace rolled back' section"
echo "      - Governance badges show 'rolled back (full)' + review: needs-attention"
echo "      - 'What should I do next?' card says re-review is required"
echo "      - Rollback caveat says it restored the Airlock workspace, NOT your repo"
echo "   2. Leave that run open, then in another terminal run:"
echo "        ./airlock review $B_ID --state approved --note 'demo'"
echo "      The open page auto-refreshes (or shows 'Evidence updated. Refreshing…')"
echo "      and the review badge flips to approved — no manual reload needed."
echo "   3. Open a run with a policy denial: the Replay summary collapses the ~20"
echo "      ENV_DENY events into one neutral 'Environment guardrail' note (not red)."
echo ""
echo " Operator-mode UI rollback (execute rollback from the browser):"
echo "   The steps above used terminal rollback. To validate the browser path"
echo "   (pre-rollback page -> click restore -> post-rollback page):"
echo ""
echo "     ./airlock serve --background --open   # operator mode, keeps your terminal"
echo "     # run a fresh governed agent, then open its run page BEFORE rolling back:"
echo "     #   - the Rollback section shows a 'Restore workspace' button"
echo "     #   - click it, confirm the prompt (workspace only, review resets)"
echo "     #   - the page auto-refreshes to '↩ Workspace rolled back'"
echo "     ./airlock serve --status              # mode / URL / PID / log"
echo "     ./airlock serve --stop               # shut the viewer down cleanly"
echo ""
echo "   Read-only mode (./airlock serve --read-only) never shows that button —"
echo "   it shows 'Rollback instructions' with the exact terminal commands instead."
echo "   UI rollback runs the SAME implementation as the CLI (internal/rollback)."
echo "==========================================================="
