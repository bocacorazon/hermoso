#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
profile=${HERMOSO_PROFILE:-"$root/profiles/default.yaml"}
mode=${1:-live}
failures=0

pass() { printf 'ok: %s\n' "$1"; }
fail() { printf 'error: %s\n' "$1" >&2; failures=$((failures + 1)); }

for path in \
  "$root/skills/hermoso/SKILL.md" \
  "$root/skills/hermoso-design/SKILL.md" \
  "$root/skills/hermoso-verification-author/SKILL.md" \
  "$root/skills/hermoso-construction/SKILL.md" \
  "$root/skills/hermoso-verification/SKILL.md" \
  "$profile"
do
  [ -f "$path" ] || fail "missing $path"
done

if command -v python3 >/dev/null 2>&1; then
  if python3 - "$profile" <<'PY'
import json, pathlib, sys
p = pathlib.Path(sys.argv[1])
d = json.loads(p.read_text())
assert d["schema_version"] == "hermoso-profile/v1"
assert d["phases"]["construction"]["worker_skills"]
assert d["phases"]["verification_contract"]["skills"]
assert d["phases"]["verification"]["skills"]
assert d["dispatch"]["tenant_strategy"] == "project-derived"
PY
  then pass "profile JSON is structurally valid"
  else fail "invalid profile: $profile"
  fi
else
  fail "python3 is required for profile validation"
fi

[ "$mode" = "--static" ] && {
  [ "$failures" -eq 0 ] || exit 1
  exit 0
}

command -v hermoso >/dev/null 2>&1 && pass "hermoso executable is available" ||
  fail "hermoso executable is not on PATH"
command -v hermes >/dev/null 2>&1 && pass "hermes executable is available" ||
  fail "hermes executable is not on PATH"

config=${HERMES_CONFIG:-"$HOME/.hermes/config.yaml"}
if [ -f "$config" ] && grep -F "$root/skills" "$config" >/dev/null 2>&1; then
  pass "skills.external_dirs references $root/skills"
else
  fail "add $root/skills to skills.external_dirs in $config"
fi

if command -v hermes >/dev/null 2>&1; then
  profiles=$(hermes profile list 2>/dev/null || true)
  for name in $(python3 - "$profile" <<'PY'
import json, sys
d=json.load(open(sys.argv[1]))
p=d["phases"]
print(" ".join(sorted(set([
 p["controller"]["profile"], p["design"]["profile"],
 p["construction"]["orchestrator_profile"], p["construction"]["worker_profile"],
 p["integration"]["profile"], p["verification_contract"]["profile"],
 p["verification"]["profile"],
]))))
PY
  ); do
    printf '%s\n' "$profiles" | grep -F "$name" >/dev/null 2>&1 &&
      pass "Hermes profile exists: $name" || fail "missing Hermes profile: $name"
  done

  hermes kanban --help >/dev/null 2>&1 &&
    pass "Hermes Kanban CLI is available" || fail "Hermes Kanban CLI is unavailable"

  installed=$(hermes skills list 2>/dev/null || true)
  for skill in kanban-orchestrator kanban-worker test-driven-development systematic-debugging spike; do
    printf '%s\n' "$installed" | grep -F "$skill" >/dev/null 2>&1 &&
      pass "required skill is visible: $skill" || fail "required skill is not visible: $skill"
  done

  hermes gateway status >/dev/null 2>&1 &&
    pass "Hermes gateway is ready" ||
    fail "Hermes gateway is not ready; start it before automatic Kanban dispatch"
fi

[ "$failures" -eq 0 ] || exit 1
