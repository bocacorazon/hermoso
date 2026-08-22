#!/usr/bin/env python3
"""Probe: check that hermoso-constitution is bound in the default profile."""
import json, os, sys

profile_path = os.path.join(os.getcwd(), "profiles", "default.yaml")
if not os.path.exists(profile_path):
    print("missing")
    sys.exit(1)

with open(profile_path) as f:
    d = json.load(f)

skills = []
for p in d.get("phases", {}).values():
    v = p.get("skills") or p.get("orchestrator_skills") or []
    if isinstance(v, list):
        skills.extend(v)

if "hermoso-constitution" in skills:
    print("has-constitution-skill")
    sys.exit(0)
else:
    print("missing")
    sys.exit(1)
