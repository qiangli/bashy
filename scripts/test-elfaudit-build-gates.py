#!/usr/bin/env python3
"""Hermetic orchestration checks: no product build or foreign suite runs."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
CASES = [
    "TestLinuxBashySignalSymbolBuildGate/retained",
    "TestLinuxBashySignalSymbolBuildGate/stripped",
    "TestDarwinBashySignalSnapshotBuildGate/product",
    "TestDarwinBashySignalSnapshotBuildGate/plain",
]
FAKE = r'''#!/usr/bin/env python3
import json, os, sys
args = sys.argv[1:]
with open(os.environ['MOCK_GO_CALLS'], 'a') as f:
    f.write(json.dumps(args) + '\n')
mode = os.environ['MOCK_GO_MODE']
if '-list' in args:
    if mode != 'discovery-empty':
        print('TestLinuxBashySignalSymbolBuildGate')
        if mode != 'discovery-missing':
            print('TestDarwinBashySignalSnapshotBuildGate')
    sys.exit(0)
name = args[args.index('-run')+1].replace('^', '').replace('$', '')
if mode == 'child-fails':
    sys.exit(7)
if mode != 'child-missing':
    print(json.dumps({'Action': 'skip' if mode == 'child-skips' else 'pass', 'Test': name}))
    if mode == 'child-duplicate':
        print(json.dumps({'Action': 'pass', 'Test': name}))
if mode != 'package-missing':
    print(json.dumps({'Action': 'pass'}))
'''

for mode in ("pass", "discovery-empty", "discovery-missing", "child-fails",
             "child-missing", "child-skips", "child-duplicate", "package-missing"):
    with tempfile.TemporaryDirectory(prefix="elfaudit-gate-mock-") as tmp:
        tmp = Path(tmp)
        (tmp / "go").write_text(FAKE)
        (tmp / "go").chmod(0o755)
        calls = tmp / "calls.jsonl"
        env = dict(os.environ, PATH=str(tmp) + os.pathsep + os.environ["PATH"],
                   EVIDENCE_DIR=str(tmp), MOCK_GO_MODE=mode, MOCK_GO_CALLS=str(calls))
        result = subprocess.run(["bash", str(ROOT / "scripts/test-elfaudit-build-gates.sh")],
                                env=env, capture_output=True, text=True, timeout=20)
        actual = [json.loads(line) for line in calls.read_text().splitlines()]
        assert (result.returncode == 0) == (mode == "pass"), (mode, result.stdout, result.stderr)
        expected_count = 5 if mode == "pass" else 1 if mode.startswith("discovery-") else 2
        assert len(actual) == expected_count, (mode, actual)
        if mode == "child-fails":
            assert result.returncode == 7, result.returncode
        if mode == "pass":
            for args, selected in zip(actual[1:], CASES):
                parent, child = selected.split("/")
                assert args == ["test", "-json", "-count=1", "-timeout=10m", "-run",
                                f"^{parent}$/^{child}$", "./tools/elfaudit"], args
        print(f"elfaudit gate mock: PASS {mode}")
