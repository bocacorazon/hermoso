#!/usr/bin/env bash
# Probe: hermoso usage --model filter
set -euo pipefail

export HERMES_TELEMETRY_DB="$(dirname "$0")/../fixtures/telemetry.fixture.db"
BIN="$(dirname "$0")/../../../../bin/hermoso"

OUTPUT="$("$BIN" usage --model qwen 2>&1)"
echo "$OUTPUT"

# Should include qwen rows
echo "$OUTPUT" | grep -q "qwen-3.8-27b" || { echo "FAIL: qwen model not found"; exit 1; }

# Should NOT include deepseek rows
if echo "$OUTPUT" | grep -q "deepseek"; then
    echo "FAIL: deepseek appeared in qwen-only output"
    exit 1
fi

echo "PASS: model filter works"