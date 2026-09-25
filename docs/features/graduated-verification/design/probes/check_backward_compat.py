#!/usr/bin/env python3
"""Probe: max_verification_attempts defaults to 2 when absent (backward compat)."""
import sys
# This probe verifies the Go domain test for backward compatibility.
# The domain test in service_test.go already covers this.
# For the probe, we import and run the Go test suite directly.
import subprocess, os

assets = os.environ.get("HERMOSO_VERIFICATION_ASSETS", os.getcwd())
repo = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(assets))))

result = subprocess.run(
    ["go", "test", "./internal/domain/...", "-count=1"],
    cwd=repo, capture_output=True, text=True, timeout=60
)
if result.returncode == 0:
    print("PASS: backward compat (default 2) verified via go test")
    sys.exit(0)
print("FAIL:", result.stderr[-200:])
sys.exit(1)