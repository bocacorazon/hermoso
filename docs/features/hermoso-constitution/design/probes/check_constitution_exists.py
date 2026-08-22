#!/usr/bin/env python3
"""Probe: check that the constitution skill SKILL.md has the required structure."""
import os, sys

skill_path = os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))), "skills", "hermoso-constitution", "SKILL.md")
# Navigate to repo root: probes/ -> design/ -> hermoso-constitution/ -> features/ -> docs/ -> repo root
repo_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))))
skill_path = os.path.join(repo_root, "skills", "hermoso-constitution", "SKILL.md")

if not os.path.exists(skill_path):
    print("missing")
    sys.exit(1)

with open(skill_path) as f:
    content = f.read()

print("exists" if os.path.exists(skill_path) else "missing")
print("has-create" if "create" in content.lower() else "no-create")
print("has-amend" if "amend" in content.lower() else "no-amend")
print("has-version" if "version" in content.lower() else "no-version")
print("has-principle" if "principle" in content.lower() else "no-principle")
print("has-hermoso-prohibition" if ".hermoso" in content.lower() and ("never edit" in content.lower() or "do not edit" in content.lower()) else "no-prohibition")
print("has-never-infer" if "never infer" in content.lower() else "no-never-infer")
print("has-hermoso-context" if "hermoso context" in content.lower() else "no-hermoso-context")
print("has-explicit" if "explicit" in content.lower() else "no-explicit")
