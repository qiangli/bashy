#!/usr/bin/env python3
"""Verify the retained Darwin signing contract without native signing services."""
import io
import os
import pathlib
import subprocess
import tarfile
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]

class DarwinSignatures(unittest.TestCase):
    def run_fixture(self, paired=False, mismatched=False):
        with tempfile.TemporaryDirectory() as work:
            work = pathlib.Path(work)
            tools = work / 'tools'
            tools.mkdir()
            for name in ('codesign', 'spctl'):
                tool = tools / name
                tool.write_text('#!/bin/bash\nprintf "%s\\n" "${@: -1}" >> "$SIGNATURE_CALLS"\n')
                tool.chmod(0o755)
            dist = work / 'dist'
            dist.mkdir()
            members = ['bashy','outpost','bash','sh'] + (['bash.real','sh.real'] if paired else [])
            for family in ('bashy','bash'):
                with tarfile.open(dist / (family + '-darwin-arm64.tar.gz'), 'w:gz') as archive:
                    for name in members:
                        if family == 'bash' and name in ('bashy','outpost'): continue
                        data = (name + (' changed' if mismatched and family == 'bash' and name == 'bash.real' else '')).encode()
                        info = tarfile.TarInfo(name)
                        info.size = len(data)
                        archive.addfile(info, io.BytesIO(data))
            (dist / 'outpost-v1.0.0-rc.99-darwin-arm64').write_bytes(b'outpost')
            calls = work / 'calls'
            result = subprocess.run(['bash', str(ROOT / 'scripts/verify-signatures.sh'), str(dist), 'darwin', 'true'], env=dict(os.environ, PATH=str(tools)+os.pathsep+os.environ['PATH'], SIGNATURE_CALLS=str(calls)), stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            names = [pathlib.Path(line).name for line in calls.read_text().splitlines()] if calls.exists() else []
            return result, names

    def test_only_supported_arm64_archive_is_required(self):
        result,names=self.run_fixture()
        self.assertEqual(result.returncode,0,result.stderr.decode())
        self.assertEqual(sorted(names),sorted(['bashy','outpost','bash','sh']*2))

    def test_native_payloads_are_verified_too(self):
        result,names=self.run_fixture(paired=True)
        self.assertEqual(result.returncode,0,result.stderr.decode())
        self.assertEqual(sorted(names),sorted(['bashy','outpost','bash','sh','bash.real','sh.real']*2))

    def test_archive_family_payload_drift_is_rejected(self):
        result,_=self.run_fixture(paired=True,mismatched=True)
        self.assertNotEqual(result.returncode,0)

if __name__=='__main__': unittest.main()
