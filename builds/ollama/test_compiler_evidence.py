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
            row = json.loads((capture / 'invocations.jsonl').read_text())
            self.assertEqual(row['cwd'], str(cwd))
            self.assertEqual({x['origin'] for x in row['inputs']}, {'source', 'go-generated'})
            generated.unlink(); local.unlink()
            args = ['python3', str(FILES / 'audit-runner.py'), 'compiler-inputs', '--capture', str(capture), '--source', str(src), '--generated', str(src), '--output', str(root / 'out.json')]
            result = subprocess.run(args, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            data = json.loads((root / 'out.json').read_text())
            self.assertEqual(data['files']['nested/generated.c'], hashlib.sha256(b'int local;').hexdigest())
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
                    '-dM': '#define __GNUC__ 14\n#define __MINGW64__ 1\n'}[args[1]]
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
