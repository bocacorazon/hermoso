#!/usr/bin/env python3
"""Probe: amendment resets the attempt budget."""
import sys, subprocess, os

assets = os.environ.get("HERMOSO_VERIFICATION_ASSETS", os.getcwd())
repo = os.path.dirname(os.path.dirname(assets))

# Verification amend tests cover reset behavior.
result = subprocess.run(
    ["go", "test", "./internal/workflow/", "-run", "TestAmend", "-count=1", "-v"],
    cwd=repo, capture_output=True, text=True, timeout=120
)
if result.returncode == 0:
    print("PASS: amend reset tests pass")
    sys.exit(0)
print("FAIL:", result.stderr[-300:] if result.stderr else result.stdout[-300:])
sys.exit(1)