#!/usr/bin/env bash
# risky_agent.sh — simulates an agent issuing a destructive shell command.
#
# Airlock governance outcome (with --approval deny-high-risk):
#   rm -rf src → ClassifyCommand() returns HardBlocked=true
#   Airlock blocks execution before the shell command runs.
#   The run exits non-zero. Status: failed, FailureClass: policy.
#   No file events are recorded (command never executed).
#
# This demonstrates the command-level governance boundary — not just
# file-write interception, but pre-execution blocking of destructive commands.
#
# Use in Airlock:
#   ./airlock run --agent generic-shell \
#     --cmd "$(cat samples/scripts/risky_agent.sh)" \
#     --repo . --policy-pack strict --approval deny-high-risk
#   (Run will exit non-zero — expected)

set -euo pipefail

# This line will never be reached when run through Airlock with deny-high-risk.
# Airlock classifies "rm -rf" as HardBlocked before passing to the shell.
rm -rf src

echo "This line is never reached under Airlock governance."
