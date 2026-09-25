#!/usr/bin/env bash
# Probe: hermoso usage when telemetry DB is missing
set -euo pipefail

# Override to a nonexistent path
export HERMES_TELEMETRY_DB="/tmp/no-such-telemetry-$(date +%s).db"
HERMOSO_BIN="$(dirname "$0")/../../../../../bin/hermoso"

OUTPUT="$("$HERMOSO_BIN" usage 2>&1)"
RC=$?
echo "$OUTPUT"

# Must exit 0 (not error)
if [ "$RC" -ne 0 ]; then
    echo "FAIL: exited with code $RC, expected 0"
    exit 1
fi

# Must mention plugin install
echo "$OUTPUT" | grep -qi "hermes-telemetry" || { echo "FAIL: no mention of hermes-telemetry plugin"; exit 1; }

echo "PASS: missing DB handled gracefully"