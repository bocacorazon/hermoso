#!/usr/bin/env python3
"""Probe: check that hermoso-design SKILL.md has constitution gate."""
import os, sys

repo_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))))
path = os.path.join(repo_root, "skills", "hermoso-design", "SKILL.md")

if not os.path.exists(path):
    print("missing")
    sys.exit(1)

with open(path) as f:
    content = f.read()

lower = content.lower()
print("has-constitution-check" if "constitution" in lower else "no-constitution-check")
print("has-block-no-constitution" if "block" in lower and "constitution" in lower else "no-block")
print("has-run-constitution-first" if "constitution" in lower and ("run" in lower or "first" in lower) else "no-run-first")
print("has-violation-check" if "violation" in lower or "violate" in lower else "no-violation-check")
print("has-block-put" if "design put" in lower and "block" in lower else "no-block-put")
print("has-principle-check" if "principle" in lower else "no-principle-check")
print("has-escape-hatch" if "escape" in lower and "hatch" in lower else "no-escape-hatch")
print("has-override" if "override" in lower else "no-override")
print("has-decisions" if "decisions" in lower else "no-decisions")
print("has-rationale" if "rationale" in lower else "no-rationale")
