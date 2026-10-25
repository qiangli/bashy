#!/usr/bin/env python3
"""Pinned Windows toolchain preparation guards; no native compiler is run."""

import hashlib
import subprocess
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))
import prepare_llvm_mingw as mingw  # noqa: E402


class PrepareLlvmMingwTest(unittest.TestCase):
    def test_release_asset_pins(self):
        self.assertEqual(mingw.VERSION, "20260616")
        self.assertEqual(mingw.ASSETS["amd64"], (
            "x86_64", 187504083,
            "b9b68a4d276e16fa25802aaba458e4638f64b3884c290aaccdc2d87083b6ca35",
        ))
        self.assertEqual(mingw.ASSETS["arm64"], (
            "aarch64", 181877750,
            "312593669435bd0bfc1a43ac3fba23c8b27e0610bade88b2738e5a01702a99ba",
        ))

    def test_verified_extraction_and_tamper_rejection(self):
        with tempfile.TemporaryDirectory() as td:
            destination = Path(td)
            folder = "llvm-mingw-20260616-ucrt-aarch64"
            archive = destination / "fixture.zip"
            with zipfile.ZipFile(archive, "w") as zf:
                for name in ("clang.exe", "clang++.exe", "llvm-objdump.exe"):
                    zf.writestr(f"{folder}/bin/{name}", b"fixture")
            digest = hashlib.sha256(archive.read_bytes()).hexdigest()
            pins = {"arm64": ("aarch64", archive.stat().st_size, digest)}
            with patch.object(mingw, "target_arch", return_value="arm64"), patch.object(mingw, "ASSETS", pins), patch.object(
                mingw, "pe_machine", return_value=0xaa64
            ), patch.object(mingw.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "aarch64-w64-windows-gnu\n", "")):
                report = mingw.prepare(destination, archive)
            self.assertEqual(report["archive_sha256"], digest)
            self.assertEqual(report["compiler_host_pe_machine"], "0xaa64")
            self.assertEqual(report["compiler_default_target"], "aarch64-w64-windows-gnu")
            self.assertTrue((Path(report["toolchain_root"]) / "bin" / "clang.exe").is_file())
            archive.write_bytes(archive.read_bytes() + b"tamper")
            with self.assertRaisesRegex(RuntimeError, "byte count mismatch"):
                mingw.verified_archive(archive, pins["arm64"][1], digest)

    def test_archive_cannot_escape_destination(self):
        with tempfile.TemporaryDirectory() as td:
            destination = Path(td)
            folder = "llvm-mingw-20260616-ucrt-x86_64"
            archive = destination / "unsafe.zip"
            with zipfile.ZipFile(archive, "w") as zf:
                zf.writestr(f"{folder}/../escape.txt", b"escape")
            with self.assertRaisesRegex(RuntimeError, "unsafe llvm-mingw archive member"):
                mingw.unpack_verified_archive(archive, destination, folder)
            self.assertFalse((destination / "escape.txt").exists())


if __name__ == "__main__":
    unittest.main()
