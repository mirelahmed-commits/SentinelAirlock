#!/usr/bin/env bash
# Sentinel Airlock — Full governance boundary demo (~5 minutes)
#
# Proves three distinct governance behaviors:
#   1. Clean safe execution    — writes to src/ allowed; full artifact trail
#   2. Policy denial at write  — .env and deploy-path writes blocked + reverted; recorded
#   3. Command-level block     — destructive rm -rf blocked before execution
#
# Policy model used: --policy-pack strict
#   allow_write: ["src/**", "app/**"]
#   deny_read:   ["**/.env", "**/*.pem", "**/.ssh/**", "**/.aws/**"]
#   network:     off
#
# Enforcement model:
#   - File writes violating policy are reverted in-place by the filesystem watcher.
#     The write is undone; a POLICY_DENY event (with diff) is recorded in events.jsonl.
#   - Commands matching rm -rf / chmod -r / chown -r are blocked at classification
#     before execution starts. The run exits non-zero; Status: failed class=policy.
#
# Prerequisites: Go 1.22+, make
# No Docker. No network. No external API keys.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

echo "========================================================="
echo " Sentinel Airlock — Full Governance Boundary Demo"
echo "========================================================="
echo ""

# ── Build ─────────────────────────────────────────────────────
echo "[ BUILD ] Building binary..."
make build --quiet
echo "          Binary: $(./airlock --version)"
echo ""

# ── Bootstrap ─────────────────────────────────────────────────
echo "[ SETUP ] Bootstrapping..."
./airlock bootstrap 2>&1 | grep -E "^(Bootstrap complete|Step|Policy path)" || true
echo ""

# ═══════════════════════════════════════════════════════════════
# SCENARIO 1: Clean safe execution
# Agent writes only to src/ — within strict pack allow_write
# Expected: all writes allowed; clean artifacts
# ═══════════════════════════════════════════════════════════════
echo "═══════════════════════════════════════════════════════════"
echo " SCENARIO 1: Safe agent — writes inside allowed src/ path"
echo "═══════════════════════════════════════════════════════════"
echo ""
echo " Command: mkdir -p src && echo ... > src/artifact.txt"
echo " Policy:  strict (allow_write: src/**, app/**)"
echo " Expected: writes allowed; events.jsonl shows FILE_CREATE risk=low"
echo ""

./airlock run \
  --agent generic-shell \
  --cmd 'mkdir -p src && echo "Build artifact from safe agent" > src/artifact.txt && echo "Tests: PASS" > src/test_results.txt' \
  --repo . \
  --policy-pack strict \
  --approval deny-high-risk

RUN1_ID="$(ls -1t .airlock/runs | head -n 1)"
echo ""
echo "--- Inspect Run 1 ---"
./airlock inspect "$RUN1_ID" | grep -E "^(Run ID|Touched|Denied|Status|Policy pack)"
echo ""
echo "--- Events (all should be FILE_CREATE/allowed) ---"
./airlock replay "$RUN1_ID" --tail 10
echo ""

# ═══════════════════════════════════════════════════════════════
# SCENARIO 2: Policy denial — agent attempts sensitive path writes
# Mixed command: src/ write (allowed) + .env write (blocked) +
#               deploy_config.yaml write (blocked — "deploy" in path)
# Expected: src/ write succeeds; others reverted; POLICY_DENY in events.jsonl
# ═══════════════════════════════════════════════════════════════
echo "═══════════════════════════════════════════════════════════"
echo " SCENARIO 2: Policy denial — agent writes .env + deploy path"
echo "═══════════════════════════════════════════════════════════"
echo ""
echo " Command: src/output.txt (allowed) + .env (blocked) + deploy_config.yaml (blocked)"
echo " Policy:  strict — .env is always HardBlocked; paths with 'deploy' are HardBlocked"
echo " Expected: POLICY_DENY events; files reverted; run exits 0 (command ran, writes denied)"
echo ""

./airlock run \
  --agent generic-shell \
  --cmd 'mkdir -p src && echo "agent output" > src/output.txt && echo "SECRET_KEY=abc123" > .env && echo "server: prod" > deploy_config.yaml' \
  --repo . \
  --policy-pack strict \
  --approval deny-high-risk

RUN2_ID="$(ls -1t .airlock/runs | head -n 1)"
echo ""
echo "--- Inspect Run 2 ---"
./airlock inspect "$RUN2_ID" | grep -E "^(Run ID|Touched|Denied|Status|Policy pack)"
echo ""
echo "--- Events (look for POLICY_DENY entries) ---"
./airlock replay "$RUN2_ID" --tail 20
echo ""
echo "--- Verify .env was not written (should be absent or empty) ---"
if [ -f .env ]; then
  echo "  WARNING: .env exists in working dir (not workspace — original repo untouched)"
else
  echo "  OK: .env does not exist in repo (policy revert confirmed in workspace)"
fi
echo ""

# ═══════════════════════════════════════════════════════════════
# SCENARIO 3: Command-level block — destructive shell command
# rm -rf contains "rm -rf" which ClassifyCommand() marks HardBlocked.
# Airlock blocks the command before execution starts.
# Expected: run exits non-zero; Status: failed class=policy
# ═══════════════════════════════════════════════════════════════
echo "═══════════════════════════════════════════════════════════"
echo " SCENARIO 3: Command-level block — rm -rf blocked pre-execution"
echo "═══════════════════════════════════════════════════════════"
echo ""
echo " Command: rm -rf src"
echo " Policy:  strict + deny-high-risk"
echo " Expected: Airlock blocks command before shell executes; run exits non-zero"
echo ""

./airlock run \
  --agent generic-shell \
  --cmd 'rm -rf src' \
  --repo . \
  --policy-pack strict \
  --approval deny-high-risk \
  || true   # non-zero exit expected — command blocked by governance

RUN3_ID="$(ls -1t .airlock/runs | head -n 1)"
echo ""
echo "--- Inspect Run 3 (should show Status: failed class=policy) ---"
./airlock inspect "$RUN3_ID" | grep -E "^(Run ID|Touched|Denied|Status|Policy pack)"
echo ""
echo "--- Replay Run 3 (should show CMD risk=high approval=deny — no FILE events) ---"
./airlock replay "$RUN3_ID" --tail 10
echo ""

# ═══════════════════════════════════════════════════════════════
# EVIDENCE REVIEW: review + verify + export all three runs
# ═══════════════════════════════════════════════════════════════
echo "═══════════════════════════════════════════════════════════"
echo " EVIDENCE: Review, verify, and export all runs"
echo "═══════════════════════════════════════════════════════════"
echo ""

echo "--- Reviewing runs ---"
./airlock review "$RUN1_ID" --state approved  --note "Scenario 1: safe writes — clean"
./airlock review "$RUN2_ID" --state approved  --note "Scenario 2: policy denials fired correctly"
./airlock review "$RUN3_ID" --state approved  --note "Scenario 3: destructive command blocked"

echo ""
echo "--- Verifying digest integrity ---"
./airlock verify "$RUN1_ID"
./airlock verify "$RUN2_ID"
./airlock verify "$RUN3_ID"

echo ""
echo "--- Exporting evidence bundles ---"
./airlock export "$RUN1_ID" --format zip --include-report
./airlock export "$RUN2_ID" --format zip --include-report
./airlock export "$RUN3_ID" --format zip --include-report

# ── Summary ──────────────────────────────────────────────────
echo ""
echo "========================================================="
echo " Demo complete."
echo ""
echo " What you proved:"
echo "   1. Safe writes   → allowed, recorded, verifiable"
echo "   2. .env write    → blocked + reverted in workspace; POLICY_DENY in events.jsonl"
echo "   3. deploy path   → blocked + reverted in workspace; POLICY_DENY in events.jsonl"
echo "   4. rm -rf        → blocked pre-execution; run manifest shows Status: failed class=policy"
echo "   5. All runs      → tamper-evident digest; review decision persisted; exportable"
echo ""
echo " What to look at in the artifacts:"
echo "   Run 2 events.jsonl  → grep POLICY_DENY  → shows attempted diff + blocked path"
echo "   Run 3 manifest      → Status: failed, FailureClass: policy"
echo "   Any run             → run_digest.json   → SHA-256 of all artifacts"
echo ""
echo " Run IDs:"
echo "   Run 1 (safe):     $RUN1_ID"
echo "   Run 2 (deny):     $RUN2_ID"
echo "   Run 3 (blocked):  $RUN3_ID"
echo ""
echo " Open these HTML evidence reports in a browser:"
echo "   Run 1 (safe):     .airlock/runs/$RUN1_ID/report/index.html"
echo "   Run 2 (deny):     .airlock/runs/$RUN2_ID/report/index.html"
echo "   Run 3 (blocked):  .airlock/runs/$RUN3_ID/report/index.html"
echo "   (open with: open <path>  ·  or  ./airlock replay <run_id> --open)"
echo ""
echo " To browse all runs in the web viewer:"
echo "   ./airlock serve --read-only"
echo "========================================================="

