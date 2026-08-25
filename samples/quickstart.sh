#!/usr/bin/env bash
# Sentinel Airlock — Quickstart (~60 seconds)
#
# Proves the core install-and-use flow:
#   build → bootstrap → governed run → inspect → verify → browse in viewer → stop
#
# Prerequisites: Go 1.22+, make. No Docker. No network. No API keys.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "========================================"
echo " Sentinel Airlock — Quickstart"
echo "========================================"
echo ""

# ── 1. Build ──────────────────────────────
echo "[ 1/5 ] Building..."
make build --quiet
echo "        $(./airlock --version)"
echo ""

# ── 2. Bootstrap ──────────────────────────
echo "[ 2/5 ] Bootstrapping project..."
./airlock bootstrap 2>&1 | grep -E "^(Bootstrap complete|Policy path|OK|WARN)" || true
echo ""

# ── 3. Governed run ───────────────────────
echo "[ 3/5 ] Running a governed agent command..."
./airlock run \
  --agent generic-shell \
  --cmd 'mkdir -p src && echo "Hello from Airlock $(date -u)" > src/hello.txt' \
  --repo .
echo ""

# ── 4. Inspect + verify ───────────────────
echo "[ 4/5 ] Inspecting artifacts and verifying digest..."
./airlock inspect latest
./airlock verify latest
echo ""

# ── 5. Viewer ────────────────────────────
echo "[ 5/5 ] Starting the operator viewer (background)..."
./airlock serve --stop >/dev/null 2>&1 || true
./airlock serve --background --open
./airlock serve --status
echo ""
echo "        Browse runs at the URL above."
echo "        Press Enter to stop the viewer."
read -r _ || true
./airlock serve --stop
echo ""

echo "========================================"
echo " Quickstart complete."
echo ""
echo " Next steps:"
echo "   cd your-project"
echo "   make install PREFIX=\$HOME/bin    # put airlock on PATH"
echo "   airlock bootstrap"
echo "   airlock run --agent generic-shell --cmd '...' --repo ."
echo "========================================"
