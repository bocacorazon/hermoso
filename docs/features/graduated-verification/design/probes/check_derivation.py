#!/usr/bin/env python3
"""Probe: derivation engine produces correct judgments from work-graph commands."""
import sys, subprocess, os

assets = os.environ.get("HERMOSO_VERIFICATION_ASSETS", os.getcwd())
repo = os.path.dirname(os.path.dirname(assets))

# Run the Go derivation tests
result = subprocess.run(
    ["go", "test", "./internal/workflow/", "-run", "TestDerive", "-count=1", "-v"],
    cwd=repo, capture_output=True, text=True, timeout=60
)
if result.returncode == 0 and "PASS" in result.stdout:
    print("PASS: derivation engine tests pass")
    sys.exit(0)
print("FAIL:", result.stderr[-300:] if result.stderr else result.stdout[-300:])
sys.exit(1)