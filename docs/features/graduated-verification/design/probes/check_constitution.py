#!/usr/bin/env python3
"""Probe: constitution.md contains the small-feature derivation clause."""
import os, sys

assets = os.environ.get("HERMOSO_VERIFICATION_ASSETS", os.getcwd())
repo = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(assets))))
path = os.path.join(repo, "docs", "constitution.md")

with open(path) as f:
    text = f.read()

checks = [
    "complexity: small" in text,
    "MAY derive their verification contract from work-graph validation commands" in text,
    "1.1.0" in text,
    "Last amended" in text,
]

if all(checks):
    print("PASS: constitution amendment verified")
    sys.exit(0)
else:
    print("FAIL: expected text not found", [i for i, c in enumerate(checks) if not c])
    sys.exit(1)