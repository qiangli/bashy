#!/usr/bin/env python3
import hashlib
import json
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]

class RemoteAssetTests(unittest.TestCase):
    def test_server_digest_seal_rejects_changes_or_missing_digest(self):
        with tempfile.TemporaryDirectory() as work:
            work = pathlib.Path(work)
            dist = work / 'dist'
            dist.mkdir()
            (dist / 'artifact').write_bytes(b'reviewed')
            assets = [{'name': 'artifact', 'state': 'uploaded', 'size': 8, 'digest': 'sha256:' + hashlib.sha256(b'reviewed').hexdigest()}]
            listing = work / 'assets.json'
            def verify():
                listing.write_text(json.dumps(assets))
                return subprocess.run(['python3', str(ROOT / 'scripts/verify-release-remote-assets.py'), str(listing), str(dist)], stdout=subprocess.PIPE, stderr=subprocess.PIPE).returncode
            self.assertEqual(verify(), 0)
            for key, wrong in [('state', 'new'), ('size', 9), ('digest', None), ('digest', 'sha256:' + '0' * 64)]:
                original = assets[0][key]
                assets[0][key] = wrong
                self.assertNotEqual(verify(), 0)
                assets[0][key] = original
            assets.append(dict(assets[0]))
            self.assertNotEqual(verify(), 0)

if __name__ == '__main__': unittest.main()
