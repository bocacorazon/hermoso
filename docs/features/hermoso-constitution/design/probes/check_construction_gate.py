#!/usr/bin/env python3
"""Probe: check that hermoso-construction SKILL.md has constitution gate."""
import os, sys

repo_root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))))
path = os.path.join(repo_root, "skills", "hermoso-construction", "SKILL.md")

if not os.path.exists(path):
    print("missing")
    sys.exit(1)

with open(path) as f:
    content = f.read()

lower = content.lower()
print("has-constitution-check" if "constitution" in lower else "no-constitution-check")
print("has-block-dispatch" if "dispatch" in lower and "block" in lower else "no-block-dispatch")
print("has-escape-hatch-ref" if "escape" in lower or "override" in lower else "no-escape-hatch-ref")
