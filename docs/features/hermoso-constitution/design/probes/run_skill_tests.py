#!/usr/bin/env python3
"""Probe: run skill tests to verify all invariants pass."""
import subprocess, sys, os

os.chdir(os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))))
os.chdir("..")  # go to repo root

result = subprocess.run(
    [sys.executable, "-c", "import subprocess,sys; r=subprocess.run(['go','test','./skills/','-v','-run','TestSkill'],capture_output=True,text=True); print(r.stdout); print(r.stderr); sys.exit(r.returncode)"],
    capture_output=True, text=True
)
print(result.stdout)
if result.stderr:
    print(result.stderr)
sys.exit(result.returncode)
