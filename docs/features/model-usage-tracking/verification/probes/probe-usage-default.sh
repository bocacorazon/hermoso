#!/usr/bin/env bash
# Probe: hermoso usage default output shows today's usage grouped by model
set -euo pipefail

# Point to the test fixture DB
export HERMES_TELEMETRY_DB="$(dirname "$0")/../fixtures/telemetry.fixture.db"
BIN="$(dirname "$0")/../../../../bin/hermoso"

OUTPUT="$("$BIN" usage 2>&1)"
echo "$OUTPUT"

# Today is 2026-09-20 per fixture — check for expected models
echo "$OUTPUT" | grep -q "deepseek/deepseek-v4-pro" || { echo "FAIL: missing deepseek-v4-pro in default output"; exit 1; }
echo "$OUTPUT" | grep -q "qwen-3.8-27b" || { echo "FAIL: missing qwen-3.8-27b in default output"; exit 1; }
echo "$OUTPUT" | grep -q "gemini-2.5-flash" || { echo "FAIL: missing gemini-2.5-flash in default output"; exit 1; }

# Local model must show $0.00
echo "$OUTPUT" | grep -q "\$0.00" || { echo "FAIL: \$0.00 not found for local model"; exit 1; }

# Last week's rows must NOT appear in default (today-only) output
if echo "$OUTPUT" | grep -q "10000"; then
    echo "FAIL: last week's rows appeared in today-only output"
    exit 1
fi

echo "PASS: default output shows today's usage with $0.00 for locals"