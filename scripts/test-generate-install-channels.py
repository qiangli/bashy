#!/usr/bin/env python3
"""Tests for generate-install-channels.py manifest generator.

Verifies identity, publisher, homepage, and hash checking behavior.
"""

import hashlib
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
GENERATOR = ROOT / "scripts" / "generate-install-channels.py"


class GenerateInstallChannelsTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dist = pathlib.Path(self.tmp.name) / "dist"
        self.out = pathlib.Path(self.tmp.name) / "out"
        self.dist.mkdir()
        self.out.mkdir()

        # Create dummy artifacts and checksums.txt
        self.archives = [
            "bashy-darwin-arm64.tar.gz",
            "bashy-darwin-amd64.tar.gz",
            "bashy-linux-arm64.tar.gz",
            "bashy-linux-amd64.tar.gz",
            "bashy-windows-arm64.zip",
            "bashy-windows-amd64.zip",
        ]
        self.hashes = {}
        checksum_lines = []
        for name in self.archives:
            content = f"dummy payload for {name}".encode("utf-8")
            digest = hashlib.sha256(content).hexdigest()
            self.hashes[name] = digest
            (self.dist / name).write_bytes(content)
            checksum_lines.append(f"{digest}  {name}")

        (self.dist / "checksums.txt").write_text("\n".join(checksum_lines) + "\n")

    def tearDown(self):
        self.tmp.cleanup()

    def run_generator(self, tag="v1.0.0", dist=None, out=None):
        dist = dist or self.dist
        out = out or self.out
        return subprocess.run(
            [sys.executable, str(GENERATOR), tag, str(dist), str(out)],
            capture_output=True,
            text=True,
        )

    def test_generator_identity_and_publisher(self):
        res = self.run_generator("v1.0.0")
        self.assertEqual(res.returncode, 0, f"Generator failed: {res.stderr}")

        # Homebrew Formula
        formula_path = self.out / "Formula" / "bashy.rb"
        self.assertTrue(formula_path.exists(), "Formula/bashy.rb missing")
        formula_text = formula_path.read_text()
        self.assertIn('homepage "https://dhnt.io"', formula_text)
        self.assertNotIn('homepage "https://github.com/qiangli/bashy"', formula_text)

        # Scoop Manifest
        scoop_path = self.out / "bucket" / "bashy.json"
        self.assertTrue(scoop_path.exists(), "bucket/bashy.json missing")
        scoop_data = json.loads(scoop_path.read_text())
        self.assertEqual(scoop_data.get("homepage"), "https://dhnt.io")

        # Winget Manifests
        winget_dir = self.out / "winget"
        self.assertTrue(winget_dir.exists(), "winget dir missing")

        # Must NOT generate qiangli.bashy manifests
        self.assertFalse((winget_dir / "qiangli.bashy.yaml").exists())
        self.assertFalse((winget_dir / "qiangli.bashy.locale.en-US.yaml").exists())
        self.assertFalse((winget_dir / "qiangli.bashy.installer.yaml").exists())

        # Must generate DHNT.bashy manifests
        v_file = winget_dir / "DHNT.bashy.yaml"
        l_file = winget_dir / "DHNT.bashy.locale.en-US.yaml"
        i_file = winget_dir / "DHNT.bashy.installer.yaml"
        self.assertTrue(v_file.exists(), "DHNT.bashy.yaml missing")
        self.assertTrue(l_file.exists(), "DHNT.bashy.locale.en-US.yaml missing")
        self.assertTrue(i_file.exists(), "DHNT.bashy.installer.yaml missing")

        v_text = v_file.read_text()
        self.assertIn("PackageIdentifier: DHNT.bashy", v_text)

        l_text = l_file.read_text()
        self.assertIn("PackageIdentifier: DHNT.bashy", l_text)
        self.assertIn("Publisher: DHNT Inc.", l_text)
        self.assertIn("PublisherUrl: https://dhnt.io", l_text)
        self.assertIn("PackageUrl: https://dhnt.io", l_text)
        self.assertNotIn("Publisher: Qiang Li", l_text)

        i_text = i_file.read_text()
        self.assertIn("PackageIdentifier: DHNT.bashy", i_text)

    def test_generator_hashes_match_checksums(self):
        res = self.run_generator("v1.0.0")
        self.assertEqual(res.returncode, 0, f"Generator failed: {res.stderr}")

        # Homebrew Formula hashes
        formula_text = (self.out / "Formula" / "bashy.rb").read_text()
        for name in [
            "bashy-darwin-arm64.tar.gz",
            "bashy-darwin-amd64.tar.gz",
            "bashy-linux-arm64.tar.gz",
            "bashy-linux-amd64.tar.gz",
        ]:
            self.assertIn(f'sha256 "{self.hashes[name]}"', formula_text)

        # Scoop hashes
        scoop_data = json.loads((self.out / "bucket" / "bashy.json").read_text())
        self.assertEqual(scoop_data["architecture"]["64bit"]["hash"], self.hashes["bashy-windows-amd64.zip"])
        self.assertEqual(scoop_data["architecture"]["arm64"]["hash"], self.hashes["bashy-windows-arm64.zip"])

        # Winget hashes (must be uppercase)
        installer_text = (self.out / "winget" / "DHNT.bashy.installer.yaml").read_text()
        self.assertIn(f'InstallerSha256: {self.hashes["bashy-windows-amd64.zip"].upper()}', installer_text)
        self.assertIn(f'InstallerSha256: {self.hashes["bashy-windows-arm64.zip"].upper()}', installer_text)

    def test_generator_mismatched_archive_hash_fails(self):
        # Tamper with an archive
        (self.dist / "bashy-darwin-arm64.tar.gz").write_bytes(b"tampered content")
        res = self.run_generator("v1.0.0")
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("archive checksum mismatch", res.stderr)

    def test_generator_corrupt_checksums_fails(self):
        # Corrupt checksums.txt with duplicate or invalid line
        (self.dist / "checksums.txt").write_text("badline\n")
        res = self.run_generator("v1.0.0")
        self.assertNotEqual(res.returncode, 0)


if __name__ == "__main__":
    unittest.main()
