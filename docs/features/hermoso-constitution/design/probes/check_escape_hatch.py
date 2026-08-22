#!/usr/bin/env python3
"""Probe: check escape-hatch mechanism in design skill."""
import os, sys

repo_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))))
path = os.path.join(repo_root, "skills", "hermoso-design", "SKILL.md")

if not os.path.exists(path):
    print("missing")
    sys.exit(1)

with open(path) as f:
    content = f.read()

lower = content.lower()
print("has-escape-hatch" if "escape" in lower and "hatch" in lower else "no-escape-hatch")
print("has-override" if "override" in lower else "no-override")
print("has-decisions" if "decisions" in lower else "no-decisions")
print("has-rationale" if "rationale" in lower else "no-rationale")
print("has-approval" if "approval" in lower or "approve" in lower else "no-approval")
