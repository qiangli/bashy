#!/usr/bin/env python3
"""A failed scanner must never become a passing local release receipt."""
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]

class ScannerStatusTests(unittest.TestCase):
    def run_scanner(self, code, finding=''):
        with tempfile.TemporaryDirectory() as work:
            work = pathlib.Path(work)
            toolchain = next(line.split()[1] for line in (ROOT / 'go.mod').read_text().splitlines() if line.startswith('toolchain '))
            fake = work / 'scanner'
            fake.write_text('''#!/usr/bin/env python3
import json, os, sys
print(json.dumps({'config': {'protocol_version': 'v1.0.0'}}))
if os.environ.get('FAKE_FINDING'):
 print(json.dumps({'finding': {'osv': 'GO-FIXTURE-1', 'trace': [{'module': 'example.org/module', 'package': 'example.org/module', 'function': 'Vulnerable'}]}}))
sys.exit(int(os.environ['FAKE_EXIT']))
''')
            fake.chmod(0o755)
            go = work / 'go'
            go.write_text('''#!/bin/bash
set -eu
case "$1" in
 env) printf '%s\\n' "$FAKE_TOOLCHAIN" ;;
 install) cp "$FAKE_SCANNER" "$GOBIN/govulncheck" ;;
 list) echo github.com/qiangli/outpost ;;
 *) exit 2 ;;
esac
''')
            go.chmod(0o755)
            env = dict(os.environ, PATH=str(work) + os.pathsep + os.environ['PATH'],
                       FAKE_SCANNER=str(fake), FAKE_TOOLCHAIN=toolchain, FAKE_EXIT=str(code), FAKE_FINDING=finding,
                       GOVULNCHECK_OUT=str(work / 'reports'))
            return subprocess.run(['bash', str(ROOT / 'scripts/govulncheck-release.sh'), str(work)], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    def test_clean_scan_passes(self):
        result = self.run_scanner(0)
        self.assertEqual(result.returncode, 0, result.stderr.decode())

    def test_header_then_scanner_failure_is_not_success(self):
        for code in (1, 2, 127):
            with self.subTest(code=code):
                result = self.run_scanner(code)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(b'govulncheck failed', result.stderr)

    def test_reachable_finding_blocks(self):
        result = self.run_scanner(3, 'reachable')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b'REACHABLE', result.stdout)

if __name__ == '__main__': unittest.main()
