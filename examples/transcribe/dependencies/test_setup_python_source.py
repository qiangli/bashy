"""Pure rejection checks for the opt-in Linux Python source setup."""

import io
import json
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch
from subprocess import CompletedProcess

sys.path.insert(0, str(Path(__file__).resolve().parent))
import setup_python_source as setup


class SourceSetupTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def test_tampered_archive_fails_before_key_or_build(self):
        archive = self.root / "Python-3.12.15.tar.xz"
        archive.write_bytes(b"changed source")
        with patch.object(setup.subprocess, "run") as process:
            with self.assertRaisesRegex(RuntimeError, "size or SHA-256"):
                setup.verify_source(archive, self.root / "missing.asc", self.root / "missing.gpg")
            process.assert_not_called()

    def test_bad_source_does_not_create_work_or_prefix(self):
        archive = self.root / "source.tar.xz"
        archive.write_bytes(b"changed source")
        work, prefix = self.root / "work", self.root / "installed"
        with patch.object(setup, "target", return_value="linux/amd64"), patch.object(
            setup, "private_path", side_effect=lambda path: path
        ), patch.object(setup, "run") as process:
            with self.assertRaisesRegex(RuntimeError, "size or SHA-256"):
                setup.setup(archive, self.root / "sig", self.root / "key", work, prefix, 1)
            self.assertFalse(work.exists())
            self.assertFalse(prefix.exists())
            process.assert_not_called()

    def test_wrong_release_key_and_bad_signature_rejected(self):
        archive = self.root / "source.tar.xz"
        archive.write_bytes(b"signed fixture")
        signature, key = self.root / "source.asc", self.root / "release.gpg"
        signature.write_bytes(b"signature fixture")
        key.write_bytes(b"key fixture")
        def run_wrong_key(args, **_):
            if "--list-keys" in args:
                return CompletedProcess(args, 0, "pub:::::::::\nfpr:::::::::BADKEY:\n", "")
            return CompletedProcess(args, 0, "", "")
        with patch.object(setup, "SOURCE_BYTES", archive.stat().st_size), patch.object(
            setup, "SOURCE_SHA256", setup.sha256(archive)
        ), patch.object(setup, "SIGNATURE_SHA256", setup.sha256(signature)
        ), patch.object(setup.subprocess, "run", side_effect=run_wrong_key):
            with self.assertRaisesRegex(RuntimeError, "fingerprint"):
                setup.verify_source(archive, signature, key)

        def run_bad_signature(args, **_):
            if "--list-keys" in args:
                return CompletedProcess(args, 0, "pub:::::::::\nfpr:::::::::" + setup.RELEASE_KEY + ":\n", "")
            if "--verify" in args:
                return CompletedProcess(args, 1, "[GNUPG:] BADSIG\n", "")
            return CompletedProcess(args, 0, "", "")
        with patch.object(setup, "SOURCE_BYTES", archive.stat().st_size), patch.object(
            setup, "SOURCE_SHA256", setup.sha256(archive)
        ), patch.object(setup, "SIGNATURE_SHA256", setup.sha256(signature)
        ), patch.object(setup.subprocess, "run", side_effect=run_bad_signature):
            with self.assertRaisesRegex(RuntimeError, "detached source signature"):
                setup.verify_source(archive, signature, key)

    def test_archive_path_and_link_escape_rejected(self):
        for name, kind in (("Python-3.12.15/../outside", "file"),
                           ("Python-3.12.15/link", "symlink")):
            with self.subTest(name=name):
                archive = self.root / (kind + ".tar.xz")
                with tarfile.open(archive, "w:xz") as tar:
                    entry = tarfile.TarInfo(name)
                    if kind == "file":
                        entry.size = 1
                        tar.addfile(entry, io.BytesIO(b"x"))
                    else:
                        entry.type = tarfile.SYMTYPE
                        entry.linkname = "/tmp/escape"
                        tar.addfile(entry)
                with self.assertRaisesRegex(RuntimeError, "unsafe path|link or special"):
                    setup.unpack(archive, self.root / (kind + "-work"))

    def test_offline_reverify_detects_changed_or_added_install_file(self):
        prefix = self.root / "installed"
        (prefix / "bin").mkdir(parents=True)
        executable = prefix / "bin/python3.12"
        executable.write_bytes(b"source built executable")
        (prefix / "LICENSE.python-3.12.15").write_text("PSF test fixture")
        (prefix / "_ssl.fixture.so").write_bytes(b"ssl fixture")
        (prefix / "_hashlib.fixture.so").write_bytes(b"hashlib fixture")
        source = {"url": setup.SOURCE_URL, "sha256": setup.SOURCE_SHA256,
                  "bytes": setup.SOURCE_BYTES, "signature_url": setup.SOURCE_URL + ".asc",
                  "signature_sha256": setup.SIGNATURE_SHA256,
                  "release_key_fingerprint": setup.RELEASE_KEY}
        with patch.object(setup, "target", return_value="linux/amd64"), patch.object(
            setup, "private_path", return_value=prefix
        ), patch.object(setup, "runtime", return_value={"version": setup.VERSION}), patch.object(
            setup, "dependencies", return_value={
                "bin/python3.12": {},
                "_ssl.fixture.so": {"/lib/libssl.so.3": {"sha256": "fixture"}},
                "_hashlib.fixture.so": {"/lib/libcrypto.so.3": {"sha256": "fixture"}},
            }
        ), patch.object(
            setup, "elf_machine", return_value="Advanced Micro Devices X86-64"
        ):
            receipt = setup.audit(prefix, source, "linux/amd64", toolchain={"compiler": "fixture"})
            (prefix / "source-build-receipt.json").write_text(json.dumps(receipt))
            self.assertEqual(setup.verify_existing(prefix)["executable"], str(executable))
            executable.write_bytes(b"changed executable")
            with self.assertRaisesRegex(RuntimeError, "installed_files"):
                setup.verify_existing(prefix)
            executable.write_bytes(b"source built executable")
            (prefix / "unexpected.so").write_bytes(b"extra module")
            with self.assertRaisesRegex(RuntimeError, "installed_files"):
                setup.verify_existing(prefix)

    def test_gpl_named_or_missing_runtime_library_rejected(self):
        prefix = self.root / "installed"
        (prefix / "bin").mkdir(parents=True)
        (prefix / "bin/python3.12").write_bytes(b"ELF fixture")
        files = {"bin/python3.12": {"sha256": setup.sha256(prefix / "bin/python3.12")}}
        with patch.object(setup.subprocess, "run", return_value=CompletedProcess(
            [], 0, "libreadline.so.8 => /usr/lib/libreadline.so.8 (0x1)\n", ""
        )):
            with self.assertRaisesRegex(RuntimeError, "forbidden"):
                setup.dependencies(prefix, files)
        with patch.object(setup.subprocess, "run", return_value=CompletedProcess(
            [], 0, "libssl.so.3 => not found\n", ""
        )):
            with self.assertRaisesRegex(RuntimeError, "unresolved"):
                setup.dependencies(prefix, files)

    def test_dedicated_prefix_is_frozen_after_receipt(self):
        prefix = self.root / "python-prefix"
        (prefix / "bin").mkdir(parents=True)
        executable = prefix / "bin/python3.12"
        executable.write_bytes(b"fixture")
        executable.chmod(0o755)
        setup.freeze_prefix(prefix)
        self.assertEqual(executable.stat().st_mode & 0o222, 0)
        self.assertEqual(prefix.stat().st_mode & 0o222, 0)


if __name__ == "__main__":
    unittest.main()
