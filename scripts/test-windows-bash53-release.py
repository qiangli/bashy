#!/usr/bin/env python3
"""Focused driver regression: release mode must never rebuild product bytes."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

GATE = Path(__file__).with_name('windows-bash53-gate.sh').resolve()

class ReleaseGateTest(unittest.TestCase):
    def test_release_pair_is_preserved(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for d in ('bin', 'product', 'tools', 'fixtures/tests'):
                (root / d).mkdir(parents=True)
            def executable(name, body):
                p = root / name
                p.write_text('#!/bin/bash\n' + body)
                p.chmod(0o755)
            executable('product/bash.exe', 'echo published-shell\n')
            executable('product/bashy.exe', '[[ ${0##*/} == coreutils.exe ]] || exit 2\necho cat\n')
            executable('tools/go', '''case "$*" in
'env GOOS') echo windows;;
'build -o bin/bash53suite.exe ./tools/bash53suite') exit 0;;
'build -o bin/bash53locales.exe ./tools/bash53locales') exit 0;;
'run ./tools/bash53fixtures -root .') echo "$PWD/fixtures";;
version) echo 'go version test';;
*) echo "UNEXPECTED BUILD: $*" >&2; exit 99;;
esac
''')
            executable('bin/bash53locales.exe', 'echo "$PWD/locales"\n')
            executable('bin/bash53suite.exe', '''case " $* " in
*' -list '*) echo fixture;;
*)
[[ "$*" == *"-bash $BASH53_RELEASE_DIR/bash.exe"* ]] || exit 90
"$BASH53_USERLAND" --list >/dev/null || exit 91
echo '  PASS  fixture  (1ms)'
echo 'Results: 1 passed, 0 failed, 0 skipped';;
esac
''')
            env = dict(os.environ, PATH=str(root/'tools')+os.pathsep+os.environ['PATH'],
                       BASH53_RELEASE_DIR=str(root/'product'))
            result = subprocess.run(['/bin/bash', str(GATE), str(root)], env=env,
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
            self.assertFalse((root/'bin/bash.exe').exists())
            self.assertFalse((root/'bin/yoke.exe').exists())
            self.assertEqual((root/'bin/coreutils.exe').read_bytes(),
                             (root/'product/bashy.exe').read_bytes())
            self.assertIn('published-shell', result.stdout)
            (root/'product/bash.exe').unlink()
            missing = subprocess.run(['/bin/bash', str(GATE), str(root)], env=env,
                                     capture_output=True, text=True)
            self.assertEqual(missing.returncode, 2, missing.stdout+missing.stderr)
            self.assertIn('missing release member:', missing.stderr)
            self.assertFalse((root/'bin/bash.exe').exists())

if __name__ == '__main__':
    unittest.main()
