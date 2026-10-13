"""Adversarial compile-time fixtures. No native compiler or runner executes."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

FILES = Path(__file__).resolve().parent / 'files'
spec = importlib.util.spec_from_file_location('windows', FILES / 'windows-compilers.py')
windows = importlib.util.module_from_spec(spec)
spec.loader.exec_module(windows)
spec = importlib.util.spec_from_file_location('evidence', FILES / 'compiler-evidence.py')
evidence = importlib.util.module_from_spec(spec)
spec.loader.exec_module(evidence)


class Evidence(unittest.TestCase):
    def test_relative_deleted_temporary_inputs_and_unknown_external(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td).resolve()
            src = root / 'source'; src.mkdir()
            cwd = src / 'nested'; cwd.mkdir()
            local = cwd / 'generated.c'; local.write_text('int local;')
            temp = root / 'private-go'; temp.mkdir()
            generated = temp / 'cgo-gcc-input-123.c'; generated.write_text('int generated;')
            capture = root / 'capture'
            cc = root / 'cc'
            cc.write_text("#!/usr/bin/env python3\nimport pathlib,sys\na=sys.argv\npathlib.Path(a[a.index('-MF')+1]).write_text('obj: generated.c ' + a[2] + '\\n')\n")
            cc.chmod(0o755)
            env = dict(os.environ, BASHY_REAL_CC=str(cc), BASHY_COMPILER_SOURCE=str(src),
                       BASHY_COMPILER_TMP=str(temp), BASHY_COMPILER_CAPTURE_DIR=str(capture))
            def compile(path):
                return subprocess.run([str(FILES / 'compiler-capture-cc'), '-c', str(path)], cwd=cwd, env=env, capture_output=True, text=True)
            self.assertEqual(compile(generated).returncode, 0)
            row = json.loads(next((capture / "invocations").glob("*.json")).read_text())
            self.assertEqual(row['cwd'], str(cwd))
            self.assertEqual({x['origin'] for x in row['inputs']}, {'source', 'go-generated'})
            # Simulate relative compiler input under Go's private work directory.
            work = temp / 'go-build123' / 'b001'; work.mkdir(parents=True)
            (work / 'generated.c').write_text('int go_generated;')
            result = subprocess.run([str(FILES / 'compiler-capture-cc'), '-c', str(generated)],
                                    cwd=work, env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            (work / 'generated.c').unlink()
            generated.unlink(); local.unlink()
            args = ['python3', str(FILES / 'audit-runner.py'), 'compiler-inputs', '--capture', str(capture), '--source', str(src), '--generated', str(src), '--output', str(root / 'out.json')]
            result = subprocess.run(args, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            data = json.loads((root / 'out.json').read_text())
            self.assertEqual(data['files']['nested/generated.c'], hashlib.sha256(b'int local;').hexdigest())
            record_path = next((capture / 'invocations').glob('*.json'))
            original = record_path.read_text()
            bad = json.loads(original); bad['inputs'].pop()
            record_path.write_text(json.dumps(bad))
            self.assertNotEqual(subprocess.run(args, capture_output=True).returncode, 0)
            record_path.write_text(original)
            blob = capture / 'content' / row['inputs'][0]['sha256']; blob.write_text('tampered')
            self.assertNotEqual(subprocess.run(args, capture_output=True).returncode, 0)
            local.write_text('int local;')
            for name in ('unknown.c', 'module-cache/native.h', 'escape/cgo-gcc-input-1.c'):
                external = temp / name; external.parent.mkdir(parents=True, exist_ok=True); external.write_text('unreviewed')
                result = compile(external)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('unreviewed external native', result.stderr)
            link = temp / 'cgo-gcc-input-2.c'; link.symlink_to(external)
            self.assertNotEqual(compile(link).returncode, 0)

    def test_windows_identity_rejects_wrong_target_clang_and_mismatched_pair(self):
        def output(args, **kwargs):
            return {'-dumpmachine': 'x86_64-w64-mingw32', '--version': 'gcc 14', '-dumpfullversion': '14.2',
                    '-dM': '#define __GNUC__ 14\n#define __MINGW64__ 1\n#define __x86_64__ 1\n#define __SIZEOF_POINTER__ 8\n#define __cplusplus 201703L\n'}[args[1]]
        with patch.object(windows.subprocess, 'check_output', side_effect=output):
            self.assertEqual(len(windows.probe(Path('/tool/gcc.exe'), Path('/tool/g++.exe'))), 2)
            with self.assertRaises(ValueError): windows.probe(Path('/tool/gcc.exe'), Path('/other/g++.exe'))
        for bad in ('x86_64-pc-msys', 'aarch64-w64-mingw32', 'x86_64-linux-gnu'):
            with patch.object(windows.subprocess, 'check_output', side_effect=lambda a, **kw: bad if a[1] == '-dumpmachine' else output(a, **kw)):
                with self.assertRaises(ValueError): windows.probe(Path('/tool/gcc.exe'), Path('/tool/g++.exe'))
        with patch.object(windows.subprocess, 'check_output', side_effect=lambda a, **kw: output(a, **kw) + ('#define __clang__ 1\n' if a[1] == '-dM' else '')):
            with self.assertRaises(ValueError): windows.probe(Path('/tool/gcc.exe'), Path('/tool/g++.exe'))

    def test_diagnostic_upload_is_always_and_release_still_requires_gates(self):
        workflow = (FILES.parent / 'fork/.github/workflows/bashy-build.yml').read_text()
        step = workflow.split('- name: upload diagnostic compiler input evidence')[1].split('- name:', 1)[0]
        self.assertIn('if: always()', step)
        self.assertIn('out/*.compiler-inputs/**', step)
        self.assertIn('out/*compiler-identity.json', step)
        self.assertIn('needs: [pin, build, audit]', workflow)
        self.assertNotIn('continue-on-error:', workflow)
        upload = workflow.split('- name: upload unsigned build outputs')[1].split('- name:', 1)[0]
        self.assertNotIn('always()', upload)

    def test_portable_origin_is_narrow(self):
        roots = {'BASHY_COMPILER_SOURCE': 'C:/src', 'BASHY_COMPILER_TMP': 'C:/temp/private',
                 'BASHY_COMPILER_GOROOT': 'C:/Go', 'BASHY_MINGW_ROOT': 'C:/msys64/ucrt64'}
        for path, expected in [('C:/src/sub/generated.c', 'source'),
                               ('C:/Go/src/runtime/cgo/gcc_libinit.c', 'go-toolchain'),
                               ('C:/temp/private/go-build123/b012/pkg.cgo2.c', 'go-generated'),
                               ('C:/temp/private/cgo-gcc-input-123.c', 'go-generated'),
                               ('C:/temp/private/go-link-123/trivial.c', 'go-generated'),
                               ('C:/msys64/ucrt64/include/stdio.h', 'system')]:
            self.assertEqual(evidence.origin(path, roots)[0], expected)
        for path in ('C:/src-other/native.c', 'C:/Go/pkg/evil.c', 'C:/Go/src/evil.c',
                     'C:/temp/cgo-gcc-input-123.c', 'C:/temp/private/evil.c',
                     'C:/temp/private/go-build123/b012/evil.h', 'C:/src/../evil.c',
                     'C:/temp/private/go-link-123/evil.c',
                     'C:/temp/private/go-link-123/nested/trivial.c',
                     'C:/msys64/other/include/evil.h', 'C:/msys64/ucrt64/bin/evil.c'):
            with self.assertRaises(ValueError, msg=path): evidence.origin(path, roots)

    def test_dependency_parser_preserves_spaces_and_continuations(self):
        with tempfile.TemporaryDirectory() as td:
            dep = Path(td) / 'input.d'
            dep.write_text('C:/out/object.o: relative\\ name.c \\\n C:/temp/generated.c\n')
            self.assertEqual(evidence.dependencies(dep), ['relative name.c', 'C:/temp/generated.c'])
            dep.write_text('obj.o: \\\r\n C:\\Users\\RUNNER~1\\AppData\\Local\\Temp\\private\\cgo-gcc-input-123.c\r\n')
            self.assertEqual(evidence.dependencies(dep),
                             ['C:/Users/RUNNER~1/AppData/Local/Temp/private/cgo-gcc-input-123.c'])
            dep.write_text('no dependency rule')
            with self.assertRaises(ValueError): evidence.dependencies(dep)

    def test_windows_identity_rejects_missing_cxx_and_version_mismatch(self):
        def output(args, **kwargs):
            if args[1] == '-dumpmachine': return 'x86_64-w64-mingw32'
            if args[1] == '--version': return 'gcc 14'
            if args[1] == '-dumpfullversion': return '15' if 'g++' in args[0] else '14'
            return '#define __GNUC__ 14\n#define __MINGW64__ 1\n#define __x86_64__ 1\n#define __SIZEOF_POINTER__ 8\n#define __cplusplus 201703L\n'
        with patch.object(windows.subprocess, 'check_output', side_effect=output):
            with self.assertRaisesRegex(ValueError, 'mismatch'):
                windows.probe(Path('/tool/gcc.exe'), Path('/tool/g++.exe'))
        with patch.object(windows.subprocess, 'check_output', side_effect=lambda a, **kw:
                          output(a, **kw).replace('#define __cplusplus 201703L\n', '')):
            with self.assertRaisesRegex(ValueError, r'C\+\+ compiler'):
                windows.probe(Path('/tool/gcc.exe'), Path('/tool/g++.exe'))

    def test_windows_discovery_records_selected_pair_and_fails_closed(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td).resolve()
            directory = root / 'ucrt64' / 'bin'
            env_file, path_file = root / 'env', root / 'path'
            def probe(cc, cxx):
                if cc.parent != directory:
                    raise ValueError('wrong target on PATH')
                return [dict(path=str(p.resolve()), target='x86_64-w64-mingw32',
                             version='gcc 14', release='14') for p in (cc, cxx)]
            previous = Path.cwd()
            try:
                os.chdir(root)
                with patch.dict(os.environ, GITHUB_ENV=str(env_file), GITHUB_PATH=str(path_file)), \
                     patch.object(windows.shutil, 'which', return_value='/wrong/gcc.exe'), \
                     patch.object(windows.Path, 'glob', return_value=[directory]), \
                     patch.object(windows, 'probe', side_effect=probe):
                    windows.main()
                    records = json.loads((root / 'out/windows-compiler-identity.json').read_text())
                    self.assertEqual(records[0]['path'], str(directory / 'gcc.exe'))
                    self.assertIn('BASHY_WINDOWS_CXX=' + str(directory / 'g++.exe'), env_file.read_text())
                    self.assertEqual(path_file.read_text().strip(), str(directory))
                    before = env_file.read_text(), path_file.read_text()
                    with patch.object(windows, 'probe', side_effect=ValueError('not native')):
                        with self.assertRaisesRegex(SystemExit, 'no verified native GCC/G\\+\\+ pair'):
                            windows.main()
                    self.assertEqual(before, (env_file.read_text(), path_file.read_text()))
            finally:
                os.chdir(previous)


if __name__ == '__main__':
    unittest.main(verbosity=2)
