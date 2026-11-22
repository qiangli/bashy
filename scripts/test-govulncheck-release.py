#!/usr/bin/env python3
"""A failed scanner or reachable finding must never become a passing local release receipt."""
import os
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]

class ScannerStatusTests(unittest.TestCase):
    def run_scanner(self, code, finding='', **extra):
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
            for key in ('RELEASE_VULN_OVERRIDE', 'RELEASE_VULN_OVERRIDE_BY'):
                env.pop(key, None)
            env.update(extra)
            result = subprocess.run(['/bin/bash', str(ROOT / 'scripts/govulncheck-release.sh'), str(work)], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            result.override_exists = (work / 'vuln-override.json').exists()
            return result

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
        self.assertIn(b'reachable vulnerabilities found', result.stderr)

    def test_retired_override_env_never_bypasses_reachable_finding(self):
        result = self.run_scanner(3, 'reachable', RELEASE_VULN_OVERRIDE='ship rc with disclosed advisories', RELEASE_VULN_OVERRIDE_BY='operator')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b'REACHABLE', result.stdout)
        self.assertFalse(result.override_exists)

    def test_hydrates_missing_yoke_external_submodules(self):
        with tempfile.TemporaryDirectory() as work:
            work = pathlib.Path(work)
            toolchain = next(line.split()[1] for line in (ROOT / 'go.mod').read_text().splitlines() if line.startswith('toolchain '))
            (work / '.gitmodules').write_text('[submodule "external/ollama/src"]\n[submodule "external/podman/src"]\n')
            fake = work / 'scanner'
            fake.write_text('#!/usr/bin/env python3\nimport json\nprint(json.dumps({"config": {"protocol_version": "v1.0.0"}}))\n')
            fake.chmod(0o755)
            go = work / 'go'
            go.write_text('#!/bin/bash\nset -eu\ncase "$1" in\n env) printf "%s\\n" "$FAKE_TOOLCHAIN" ;;\n install) cp "$FAKE_SCANNER" "$GOBIN/govulncheck" ;;\n list) test -f external/ollama/src/go.mod && test -f external/podman/src/go.mod && echo github.com/qiangli/yoke ;;\n *) exit 2 ;;\nesac\n')
            go.chmod(0o755)
            git_log = work / 'git.log'
            git_bin = work / 'git'
            git_bin.write_text('#!/bin/bash\nset -eu\nprintf "%s\\n" "$*" >> "$FAKE_GIT_LOG"\nmkdir -p "$2/external/ollama/src" "$2/external/podman/src"\n: > "$2/external/ollama/src/go.mod"\n: > "$2/external/podman/src/go.mod"\n')
            git_bin.chmod(0o755)
            env = dict(os.environ, PATH=str(work) + os.pathsep + os.environ['PATH'],
                       FAKE_SCANNER=str(fake), FAKE_TOOLCHAIN=toolchain, FAKE_GIT_LOG=str(git_log),
                       GOVULNCHECK_OUT=str(work / 'reports'))
            for key in ('RELEASE_VULN_OVERRIDE', 'RELEASE_VULN_OVERRIDE_BY'):
                env.pop(key, None)
            first = subprocess.run(['/bin/bash', str(ROOT / 'scripts/govulncheck-release.sh'), str(work)], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            self.assertEqual(first.returncode, 0, first.stderr.decode())
            self.assertEqual(git_log.read_text().splitlines(), [f'-C {work} submodule update --init external/ollama/src external/podman/src'])
            second = subprocess.run(['/bin/bash', str(ROOT / 'scripts/govulncheck-release.sh'), str(work)], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            self.assertEqual(second.returncode, 0, second.stderr.decode())
            self.assertEqual(len(git_log.read_text().splitlines()), 1)

if __name__ == '__main__': unittest.main()
