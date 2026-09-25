#!/usr/bin/env python3
"""Probe: max_verification_attempts=1 blocks, =4 allows remediation."""
import sys, subprocess, os

assets = os.environ.get("HERMOSO_VERIFICATION_ASSETS", os.getcwd())
repo = os.path.dirname(os.path.dirname(assets))

# The workflow tests cover max_verification_attempts enforcement.
result = subprocess.run(
    ["go", "test", "./internal/workflow/", "-run", "TestRunVerification", "-count=1", "-v"],
    cwd=repo, capture_output=True, text=True, timeout=120
)
if result.returncode == 0:
    print("PASS: attempt cap enforcement tests pass")
    sys.exit(0)
print("FAIL:", result.stderr[-300:] if result.stderr else result.stdout[-300:])
sys.exit(1)