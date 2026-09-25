#!/usr/bin/env bash
# Probe: hermoso usage --from / --to date range
set -euo pipefail

export HERMES_TELEMETRY_DB="$(dirname "$0")/../fixtures/telemetry.fixture.db"
BIN="$(dirname "$0")/../../../../bin/hermoso"

OUTPUT="$("$BIN" usage --from 2026-09-18 --to 2026-09-19 2>&1)"
echo "$OUTPUT"

# Should include sept 18-19 rows
echo "$OUTPUT" | grep -q "gemini-2.5-flash" || { echo "FAIL: missing gemini (sept 18)"; exit 1; }
echo "$OUTPUT" | grep -q "1000" || { echo "FAIL: missing 1000 tokens (sept 19)"; exit 1; }

# Should NOT include sept 20 rows (out of range)
# Check that the 10K token row (sept 13) is absent
if echo "$OUTPUT" | grep -q "10000"; then
    echo "FAIL: sept 13 rows appeared in sept 18-19 range"
    exit 1
fi

echo "PASS: date range filtering works"