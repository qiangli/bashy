#!/usr/bin/env python3
"""Offline release fixtures: independent roles, native pairs and asset envelope."""
import importlib.util
import json
from pathlib import Path
import tarfile
import tempfile
import threading
import unittest
from unittest.mock import patch
import zipfile
import sys
sys.path.insert(0, str(Path(__file__).resolve().parent))
import dist_package as packaging
import dist as driver

PLATFORMS = ['linux/amd64', 'linux/arm64', 'darwin/arm64', 'windows/amd64', 'windows/arm64']
OTEL = PLATFORMS[:-1]

class DistFixtures(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root/'bashy'
        self.outpost = self.root/'outpost'
        for source in (self.source, self.outpost):
            source.mkdir()
            (source/'LICENSE').write_text('fixture license')
        (self.source/'README.md').write_text('fixture README')
        (self.source/'scripts').mkdir()
        (self.source/'scripts/linux-package-postinstall.sh').write_text('#!/bin/sh\ntrue\n')
        self.stage = self.root/'products'
        for target in PLATFORMS:
            goos, arch = target.split('/')
            stage = self.stage/goos/arch
            stage.mkdir(parents=True)
            for role in packaging.ROLES:
                suffix = '.exe' if goos == 'windows' else ''
                (stage/(role+suffix)).write_text(role)
            if target == 'linux/amd64':
                for role in ('bash', 'sh'):
                    (stage/(role+'.real')).write_text(role)
                    (stage/role).write_text('native launcher')
            if goos == 'linux':
                (stage/'bashy-scratch').write_text('separate scratch')
            if target in OTEL:
                (stage/('otel.exe' if goos == 'windows' else 'otel')).write_text('otel')

    def inspect(self, path):
        role = path.read_text()
        return '\tpath\t'+packaging.MAIN.get(role, 'not-a-go-product')+'\n'

    def test_asset_set_and_actual_native_pairs(self):
        def nfpm(command, check):
            Path(command[-1]).write_bytes(b'package fixture')
        with patch.object(packaging.subprocess, 'run', side_effect=nfpm):
            layouts, assets = packaging.package(self.source, self.outpost, self.stage,
                self.root/'dist', 'v1.0.0-rc.38-dev', PLATFORMS, OTEL, 'nfpm', inspect=self.inspect)
        self.assertEqual(len(assets), 34)
        self.assertEqual([asset['name'] for asset in assets], sorted(asset['name'] for asset in assets))
        directory = self.root/'dist/assets'
        lines = (directory/'checksums.txt').read_text().splitlines()
        self.assertEqual(len(lines), 33)
        self.assertEqual([line.split('  ', 1)[1] for line in lines], sorted(line.split('  ', 1)[1] for line in lines))
        with tarfile.open(directory/'bashy-linux-amd64.tar.gz') as archive:
            self.assertEqual(set(archive.getnames()), {'README.md','LICENSE','LICENSE.outpost','bashy','outpost','bash','sh','bash.real','sh.real'})
        with tarfile.open(directory/'bash-linux-amd64.tar.gz') as archive:
            self.assertEqual(set(archive.getnames()), {'README.md','LICENSE','bash','sh','bash.real','sh.real'})
        with zipfile.ZipFile(directory/'bash-windows-arm64.zip') as archive:
            self.assertEqual(set(archive.namelist()), {'README.md','LICENSE','bash.exe','sh.exe'})
        self.assertNotIn('bash.real', layouts['darwin/arm64'])
        for asset in assets:
            self.assertEqual(asset['sha256'], packaging.digest(directory/asset['name']))
        self.assertFalse((directory/'release-assets.json').exists())

    def test_parallel_packaging_is_bounded_and_publishes_only_completed_files(self):
        lock, overlap = threading.Lock(), threading.Event()
        active, peak = 0, 0
        real_archive = packaging.archive
        def archive(destination, files):
            nonlocal active, peak
            with lock:
                active += 1
                peak = max(peak, active)
                if active >= 2:
                    overlap.set()
            try:
                if not overlap.wait(2):
                    raise AssertionError('independent archives ran serially')
                real_archive(destination, files)
            finally:
                with lock:
                    active -= 1
        def nfpm(command, check):
            Path(command[-1]).write_bytes(b'complete package fixture')
        with patch.object(packaging, 'archive', side_effect=archive), patch.object(packaging.subprocess, 'run', side_effect=nfpm):
            _, assets = packaging.package(self.source, self.outpost, self.stage,
                self.root/'parallel', 'v1.0.0-rc.38-dev', PLATFORMS, OTEL, 'nfpm', inspect=self.inspect)
        self.assertGreaterEqual(peak, 2)
        self.assertLessEqual(peak, 4)
        self.assertEqual(active, 0)
        self.assertEqual(len(assets), 34)
        for asset in assets:
            self.assertEqual(asset['sha256'], packaging.digest(self.root/'parallel/assets'/asset['name']))

    def test_parallel_archive_failure_prevents_checksum_seal(self):
        with patch.object(packaging, 'archive', side_effect=OSError('archive fixture failure')):
            with self.assertRaisesRegex(OSError, 'archive fixture failure'):
                packaging.package(self.source, self.outpost, self.stage,
                    self.root/'failed', 'v1.0.0-rc.38-dev', PLATFORMS, OTEL, 'nfpm', inspect=self.inspect)
        self.assertFalse((self.root/'failed/assets/checksums.txt').exists())

    def test_rejects_missing_swapped_and_merged_roles(self):
        stage = self.stage/'linux/amd64'
        (stage/'bash.real').unlink()
        with self.assertRaisesRegex(ValueError, 'wrong independent'):
            packaging.members(stage, 'linux', self.inspect)
        (stage/'bash.real').write_text('sh')
        with self.assertRaisesRegex(ValueError, 'wrong independent'):
            packaging.members(stage, 'linux', self.inspect)
        (stage/'bash.real').write_text('bashy')
        with self.assertRaisesRegex(ValueError, 'wrong independent'):
            packaging.members(stage, 'linux', self.inspect)

    def test_packages_private_payloads_only_frontends_in_path(self):
        config = packaging.nfpm_config(self.stage/'linux/amd64', self.source, self.outpost, '1.0.0-rc.38-dev', 'amd64')
        self.assertEqual(config['license'], 'BSD-3-Clause AND MIT')
        entries = config['contents']
        self.assertEqual({x['dst'] for x in entries if x.get('type') == 'symlink'}, {'/usr/bin/bashy','/usr/bin/outpost'})
        for role in (*packaging.ROLES, 'bash.real', 'sh.real'):
            self.assertIn('/usr/lib/bashy/bin/'+role, [x['dst'] for x in entries])
        self.assertTrue(config['scripts']['postinstall'].endswith('scripts/linux-package-postinstall.sh'))
        self.assertEqual(config['overrides']['apk']['depends'], ['gcompat'])

    def test_inputs_require_frozen_independent_sources(self):
        inputs = {'tag': 'v1.0.0-rc.38-dev', 'sources': {role: {'path':role,'commit':'a'*40} for role in driver.SOURCES}}
        file = self.root/'inputs.json'
        file.write_text(json.dumps(inputs))
        self.assertEqual(len(driver.load_inputs(file)[1]), 4)
        inputs['sources']['bash']['path'] = 'bashy'
        file.write_text(json.dumps(inputs))
        with self.assertRaisesRegex(ValueError, 'independent'):
            driver.load_inputs(file)
        inputs['sources']['bash']['path'] = '../bash'
        file.write_text(json.dumps(inputs))
        with self.assertRaisesRegex(ValueError, 'within'):
            driver.load_inputs(file)

    def test_rejects_other_source_or_platform(self):
        expected = {role: "a" * 40 for role in packaging.ROLES}
        def info(path):
            return self.inspect(path) + "\tbuild\tvcs.revision=" + "a" * 40 + "\n\tbuild\tvcs.modified=false\n\tbuild\tGOOS=linux\n\tbuild\tGOARCH=amd64\n"
        stage = self.stage/"linux/amd64"
        packaging.members(stage, "linux", info, expected, "amd64")
        with self.assertRaisesRegex(ValueError, "source/platform"):
            packaging.members(stage, "linux", info, expected, "arm64")
        expected["bash"] = "b" * 40
        with self.assertRaisesRegex(ValueError, "source/platform"):
            packaging.members(stage, "linux", info, expected, "amd64")

    def test_runner_profile_excludes_only_unavailable_arm64(self):
        self.assertEqual(set(PLATFORMS)-driver.SIGNED_RUNNERS, {'windows/arm64'})

if __name__ == '__main__':
    unittest.main()
