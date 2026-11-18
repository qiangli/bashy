#!/usr/bin/env python3
"""Offline fixture checks for the reduced supported release platform set."""
import hashlib
import io
import json
import os
import pathlib
import subprocess
import tarfile
import tempfile
import unittest
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]

class ReleaseAssetTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dist = pathlib.Path(self.tmp.name)
        self.tag = 'v1.0.0-rc.99-dev'
        base = self.tag.removesuffix('-dev')
        platforms = json.loads((ROOT / 'release/platforms.json').read_text())
        for platform in platforms['platforms']:
            system, arch = platform.split('/')
            suffix = '.exe' if system == 'windows' else ''
            for family, members in [('bashy', ('bashy', 'outpost', 'bash', 'sh')), ('bash', ('bash', 'sh'))]:
                extension = 'zip' if system == 'windows' else 'tar.gz'
                path = self.dist / f'{family}-{system}-{arch}.{extension}'
                if extension == 'zip':
                    with zipfile.ZipFile(path, 'w') as archive:
                        for name in members: archive.writestr(name + suffix, b'fixture')
                else:
                    with tarfile.open(path, 'w:gz') as archive:
                        for name in members:
                            info = tarfile.TarInfo(name)
                            info.size = 7
                            archive.addfile(info, io.BytesIO(b'fixture'))
            raw = self.dist / f'outpost-{base}-{system}-{arch}{suffix}'
            raw.write_bytes(b'fixture executable')
            (self.dist / (raw.name + '.sha256')).write_text(hashlib.sha256(raw.read_bytes()).hexdigest() + '  ' + raw.name + '\n')
        for arch in ('amd64', 'arm64'):
            for extension in ('apk', 'deb', 'rpm'):
                (self.dist / f'bashy_{self.tag[1:]}_linux_{arch}.{extension}').write_bytes(b'fixture package')
            (self.dist / f'bashy-scratch-linux-{arch}').write_bytes(b'fixture scratch')
        for platform in platforms['otel_platforms']:
            (self.dist / ('otel-' + platform.replace('/', '-') + '.gz')).write_bytes(b'fixture otel')
        (self.dist / 'otel-blob-NOTICE.txt').write_bytes(b'fixture notice')
        self.seal()

    def seal(self):
        (self.dist / 'checksums.txt').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + p.name + '\n' for p in sorted(self.dist.iterdir()) if p.name != 'checksums.txt'))

    def run_gate(self):
        return subprocess.run(['bash', str(ROOT / 'scripts/verify-release-asset-set.sh'), str(self.dist)], env=dict(os.environ, TAG=self.tag), stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    def test_five_platforms_pass_with_34_assets(self):
        self.assertEqual(len(list(self.dist.iterdir())), 34)
        result = self.run_gate()
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertIn(b'34 files and 33 checksums', result.stdout)

    def test_retired_intel_archive_is_rejected(self):
        (self.dist / 'bashy-darwin-amd64.tar.gz').write_bytes(b'retired')
        self.seal()
        self.assertNotEqual(self.run_gate().returncode, 0)

    def test_missing_arm64_macos_is_rejected(self):
        (self.dist / 'bashy-darwin-arm64.tar.gz').unlink()
        self.seal()
        self.assertNotEqual(self.run_gate().returncode, 0)

    def test_native_launcher_requires_its_payload(self):
        path = self.dist / 'bash-linux-amd64.tar.gz'
        for include_payload in (False, True):
            with tarfile.open(path, 'w:gz') as archive:
                for name, body in [('bash', b'\x7fELF native fixture'), ('sh', b'plain Go fixture')] + ([('bash.real', b'Go payload')] if include_payload else []):
                    info = tarfile.TarInfo(name)
                    info.size = len(body)
                    archive.addfile(info, io.BytesIO(body))
            self.seal()
            result = self.run_gate()
            self.assertEqual(result.returncode == 0, include_payload, result.stderr.decode())

    def test_checksum_tampering_is_rejected(self):
        (self.dist / 'bashy-scratch-linux-amd64').write_bytes(b'changed')
        self.assertNotEqual(self.run_gate().returncode, 0)

    def test_sidecar_must_bind_its_binary(self):
        (self.dist / 'outpost-v1.0.0-rc.99-linux-amd64.sha256').write_text('0' * 64 + '  outpost-v1.0.0-rc.99-linux-amd64\n')
        self.seal()
        self.assertNotEqual(self.run_gate().returncode, 0)

if __name__ == '__main__': unittest.main()
