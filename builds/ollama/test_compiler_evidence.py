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
SHARED = Path(__file__).resolve().parent.parent / '_shared'
# The Windows probe is shared with prima.cpp via builds/_shared and only sits
# beside compiler-identity.py in the rendered fork.
spec = importlib.util.spec_from_file_location('windows', SHARED / 'windows-compilers.py')
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

    def test_windows_probe_accepts_mingw_clang_pair(self):
        # Shared probe (builds/_shared, prima.cpp-proven): the MinGW clang
        # pair targets x86_64-w64-windows-gnu. An MSVC-target or non-clang
        # compiler, or a mismatched pair, is refused.
        macros = ('#define __clang__ 1\n#define __MINGW64__ 1\n#define __x86_64__ 1\n'
                  '#define __SIZEOF_POINTER__ 8\n#define __cplusplus 202002L\n')
        def output(args, **kwargs):
            return {'-dumpmachine': 'x86_64-w64-windows-gnu', '--version': 'clang version 19.1.7',
                    '-dM': macros}[args[1]]
        with patch.object(windows.subprocess, 'check_output', side_effect=output):
            records = windows.probe(Path('/tool/clang.exe'), Path('/tool/clang++.exe'))
            self.assertEqual([r['target'] for r in records], ['x86_64-w64-windows-gnu'] * 2)
            with self.assertRaises(ValueError):
                windows.probe(Path('/tool/clang.exe'), Path('/other/clang++.exe'))
        for bad in ('x86_64-pc-windows-msvc', 'x86_64-pc-msys', 'x86_64-linux-gnu'):
            with patch.object(windows.subprocess, 'check_output', side_effect=lambda a, **kw: bad if a[1] == '-dumpmachine' else output(a, **kw)):
                with self.assertRaises(ValueError): windows.probe(Path('/tool/clang.exe'), Path('/tool/clang++.exe'))
        with patch.object(windows.subprocess, 'check_output', side_effect=lambda a, **kw: output(a, **kw).replace('#define __clang__ 1\n', '')):
            with self.assertRaises(ValueError): windows.probe(Path('/tool/clang.exe'), Path('/tool/clang++.exe'))
        with patch.object(windows.subprocess, 'check_output', side_effect=lambda a, **kw: output(a, **kw) + '#define _MSC_VER 1930\n' if a[1] == '-dM' else output(a, **kw)):
            with self.assertRaises(ValueError): windows.probe(Path('/tool/clang.exe'), Path('/tool/clang++.exe'))

    def test_diagnostic_upload_is_always_and_release_still_requires_gates(self):
        workflow = (FILES.parent / 'fork/.github/workflows/bashy-build.yml').read_text()
        step = workflow.split('- name: upload diagnostic compiler input evidence')[1].split('- name:', 1)[0]
        self.assertIn('if: always()', step)
        self.assertIn('out/*.compiler-inputs/**', step)
        self.assertIn('out/*compiler-identity.json', step)
        self.assertIn('needs: [pin, build, audit]', workflow)
        self.assertNotIn('continue-on-error:', workflow)
        upload = workflow.split('- name: upload unsigned build outputs')[1].split('- name:', 1)[0]
        self.assertIn('if: always()', upload)

    def test_persist_shared_blob_under_parallel_writers(self):
        # Parallel cgo compilations snapshot identical headers at once and
        # race on one blob name. Every writer must land the exact bytes with
        # no temp-file litter, and a complete blob must be reused in place.
        import concurrent.futures
        with tempfile.TemporaryDirectory() as td:
            store = Path(td)
            content = b'parallel header bytes'
            digest = hashlib.sha256(content).hexdigest()
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                list(pool.map(lambda _: evidence._persist(store, digest, content), range(32)))
            self.assertEqual((store / digest).read_bytes(), content)
            self.assertEqual(sorted(p.name for p in store.iterdir()), [digest])
            os.utime(store / digest, (0, 0))
            evidence._persist(store, digest, content)
            self.assertEqual((store / digest).stat().st_mtime_ns, 0)

    def test_persist_retries_locked_replace_with_backoff(self):
        # Windows file locking refuses os.replace onto a blob another writer
        # holds open ([WinError 5]): the store must back off and retry, never
        # fail the compile on the first locked attempt.
        with tempfile.TemporaryDirectory() as td:
            store = Path(td)
            content = b'locked blob bytes'
            digest = hashlib.sha256(content).hexdigest()
            calls = []
            real_replace = os.replace
            def flaky(src, dst):
                calls.append((src, dst))
                if len(calls) == 1:
                    raise PermissionError(13, 'denied')
                return real_replace(src, dst)
            with patch.object(os, 'replace', side_effect=flaky):
                with patch('time.sleep') as nap:
                    evidence._persist(store, digest, content)
            self.assertEqual((store / digest).read_bytes(), content)
            self.assertEqual(len(calls), 2)
            nap.assert_called_once()

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

    def test_windows_probe_rejects_missing_cxx_and_version_mismatch(self):
        macros = ('#define __clang__ 1\n#define __MINGW64__ 1\n#define __x86_64__ 1\n'
                  '#define __SIZEOF_POINTER__ 8\n#define __cplusplus 202002L\n')
        def output(args, **kwargs):
            if args[1] == '-dumpmachine': return 'x86_64-w64-windows-gnu'
            if args[1] == '--version': return 'clang version 20.1' if 'clang++' in args[0] else 'clang version 19.1.7'
            return macros
        with patch.object(windows.subprocess, 'check_output', side_effect=output):
            with self.assertRaisesRegex(ValueError, 'mismatch'):
                windows.probe(Path('/tool/clang.exe'), Path('/tool/clang++.exe'))
        with patch.object(windows.subprocess, 'check_output', side_effect=lambda a, **kw:
                          output(a, **kw).replace('#define __cplusplus 202002L\n', '')):
            with self.assertRaisesRegex(ValueError, r'C\+\+ compiler'):
                windows.probe(Path('/tool/clang.exe'), Path('/tool/clang++.exe'))

    def test_windows_discovery_uses_clang64_only_and_records_toolchain(self):
        # The shared probe never consults PATH: only <msys2-location>/clang64/bin.
        self.assertNotIn('import shutil', (SHARED / 'windows-compilers.py').read_text())
        macros = ('"#define __clang__ 1" "#define __MINGW64__ 1" '
                  '"#define __x86_64__ 1" "#define __SIZEOF_POINTER__ 8" '
                  '"#define __cplusplus 202002L"')
        with tempfile.TemporaryDirectory() as msys, tempfile.TemporaryDirectory() as work:
            clang64 = Path(msys) / 'clang64' / 'bin'
            clang64.mkdir(parents=True)
            for name in ('clang.exe', 'clang++.exe'):
                script = clang64 / name
                script.write_text('#!/bin/sh\n'
                                  'if [ "$1" = -dumpmachine ]; then echo "x86_64-w64-windows-gnu"; exit 0; fi\n'
                                  'if [ "$1" = --version ]; then echo "clang version 19.1.7"; exit 0; fi\n'
                                  f'printf "%s\\n" {macros}\n')
                script.chmod(0o755)
            for name in ('cmake.exe', 'mingw32-make.exe', 'ninja.exe', 'llvm-objdump.exe'):
                tool = clang64 / name
                tool.write_text('#!/bin/sh\necho "fixture 1.0"\n')
                tool.chmod(0o755)
            (clang64 / 'libomp.dll').write_bytes(b'fixture')
            env_file, path_file = Path(work) / 'env', Path(work) / 'path'
            env_file.write_text('')
            path_file.write_text('')
            old = {key: os.environ.get(key) for key in
                   ('PATH', windows.MSYS2_LOCATION_ENV, 'GITHUB_PATH', 'GITHUB_ENV')}
            os.environ[windows.MSYS2_LOCATION_ENV] = msys
            os.environ['GITHUB_PATH'] = str(path_file)
            os.environ['GITHUB_ENV'] = str(env_file)
            cwd = Path.cwd()
            try:
                os.chdir(work)
                windows.main()
            finally:
                os.chdir(cwd)
                for key, value in old.items():
                    if value is None:
                        os.environ.pop(key, None)
                    else:
                        os.environ[key] = value
            pinned = clang64.resolve()
            records = json.loads((Path(work) / 'out' / 'windows-compiler-identity.json').read_text())
            self.assertEqual([r['target'] for r in records], ['x86_64-w64-windows-gnu'] * 2)
            toolchain = json.loads((Path(work) / 'out' / 'windows-toolchain.json').read_text())
            self.assertEqual(toolchain['schema'], 'bashy-windows-toolchain-v1')
            self.assertEqual(toolchain['generator'], 'MinGW Makefiles')
            self.assertEqual(toolchain['make_program'], str(pinned / 'mingw32-make.exe'))
            env = env_file.read_text()
            for var, exe in [('BASHY_WINDOWS_CC', 'clang.exe'), ('BASHY_WINDOWS_CXX', 'clang++.exe'),
                             ('BASHY_WINDOWS_CMAKE', 'cmake.exe'), ('BASHY_WINDOWS_MAKE', 'mingw32-make.exe'),
                             ('BASHY_WINDOWS_NINJA', 'ninja.exe'),
                             ('BASHY_WINDOWS_OBJDUMP', 'llvm-objdump.exe')]:
                self.assertIn(f'{var}={pinned}', env)
                self.assertIn(exe, env)
            self.assertIn('BASHY_MINGW_ROOT=', env)
            self.assertIn('BASHY_TOOLCHAIN_JSON=', env)
            self.assertEqual(path_file.read_text().strip(), str(pinned))

    def test_windows_discovery_fails_closed_without_clang64(self):
        with tempfile.TemporaryDirectory() as msys, tempfile.TemporaryDirectory() as work:
            env_file, path_file = Path(work) / 'env', Path(work) / 'path'
            env_file.write_text('')
            path_file.write_text('')
            old = {key: os.environ.get(key) for key in
                   (windows.MSYS2_LOCATION_ENV, 'GITHUB_PATH', 'GITHUB_ENV')}
            os.environ[windows.MSYS2_LOCATION_ENV] = msys
            os.environ['GITHUB_PATH'] = str(path_file)
            os.environ['GITHUB_ENV'] = str(env_file)
            cwd = Path.cwd()
            try:
                os.chdir(work)
                with self.assertRaisesRegex(SystemExit, 'no verified native MinGW clang pair'):
                    windows.main()
            finally:
                os.chdir(cwd)
                for key, value in old.items():
                    if value is None:
                        os.environ.pop(key, None)
                    else:
                        os.environ[key] = value
            self.assertEqual(env_file.read_text(), '')
            self.assertEqual(path_file.read_text(), '')


if __name__ == '__main__':
    unittest.main(verbosity=2)
