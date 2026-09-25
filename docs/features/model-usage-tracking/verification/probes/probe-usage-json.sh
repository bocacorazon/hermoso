#!/usr/bin/env bash
# Probe: hermoso usage --json output
set -euo pipefail

export HERMES_TELEMETRY_DB="$(dirname "$0")/../fixtures/telemetry.fixture.db"
BIN="$(dirname "$0")/../../../../bin/hermoso"

OUTPUT="$("$BIN" usage --json 2>&1)"
echo "$OUTPUT"

# Must be valid JSON with ok, command, and data.usage
echo "$OUTPUT" | python3 -c "
import json, sys
data = json.load(sys.stdin)
assert data['ok'] is True, 'ok not true'
assert data['command'] == 'usage', f'command not usage: {data.get(\"command\")}'
assert 'usage' in data['data'], 'data.usage missing'
assert isinstance(data['data']['usage'], list), 'data.usage not a list'
assert len(data['data']['usage']) > 0, 'data.usage is empty'
print('PASS: JSON output valid with usage array')
" || { echo "FAIL: JSON validation failed"; exit 1; }

echo "PASS: JSON output"