"""Fixture checks for installed Python's actual Windows VC module admission."""

import hashlib
import json
from pathlib import Path
import tempfile
import unittest

import probe_python_loaded_vc as probe


def digest(data):
    return hashlib.sha256(data).hexdigest()


class LoadedWindowsVCTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        root = Path(self.temp.name)
        self.prefix = root / "Python"
        self.prefix.mkdir()
        self.python = self.prefix / "python.exe"
        self.python.write_bytes(b"selected interpreter")
        self.system_root = root / "Windows"
        system32 = self.system_root / "System32"
        system32.mkdir(parents=True)
        self.msvcp = system32 / "MSVCP140.dll"
        self.vcruntime = self.prefix / "VCRUNTIME140.dll"
        self.msvcp.write_bytes(b"reviewed system32 msvcp")
        self.vcruntime.write_bytes(b"reviewed app local vcruntime")
        self.loaded = {"MSVCP140.dll": self.msvcp,
                       "VCRUNTIME140.dll": self.vcruntime}
        self.policy = {"schema": "s503-windows-external-runtime-v1", "targets": {
            "AMD64": {"system32": {"MSVCP140.dll": {"sha256": digest(self.msvcp.read_bytes())}},
                      "python_loaded": {
                          "MSVCP140.dll": {"owner": "Windows System32", "sha256": digest(self.msvcp.read_bytes())},
                          "VCRUNTIME140.dll": {"owner": "Python install directory", "sha256": digest(self.vcruntime.read_bytes())}}}}}

    def verify(self):
        return probe.verify(self.policy, "AMD64", self.python, self.system_root, self.loaded)

    def test_exact_loaded_paths_and_hashes_pass(self):
        report = self.verify()
        self.assertEqual(report["VCRUNTIME140.dll"]["path"], str(self.vcruntime.resolve()))
        self.assertEqual(set(report), set(probe.NAMES))

    def test_same_bytes_at_unreviewed_path_fail(self):
        alternate = self.system_root / "System32" / "VCRUNTIME140.dll"
        alternate.write_bytes(self.vcruntime.read_bytes())
        self.loaded["VCRUNTIME140.dll"] = alternate
        with self.assertRaisesRegex(RuntimeError, "loaded from an unreviewed path"):
            self.verify()
        self.loaded["VCRUNTIME140.dll"] = self.vcruntime
        alternate_msvcp = self.prefix / "MSVCP140.dll"
        alternate_msvcp.write_bytes(self.msvcp.read_bytes())
        self.loaded["MSVCP140.dll"] = alternate_msvcp
        with self.assertRaisesRegex(RuntimeError, "loaded from an unreviewed path"):
            self.verify()

    def test_changed_app_local_bytes_fail(self):
        self.vcruntime.write_bytes(b"changed app local vcruntime")
        with self.assertRaisesRegex(RuntimeError, "VCRUNTIME140.dll loaded SHA-256"):
            self.verify()

    def test_missing_or_unreviewed_loaded_pin_fails(self):
        self.loaded.pop("VCRUNTIME140.dll")
        with self.assertRaisesRegex(RuntimeError, "both loaded VC modules"):
            self.verify()
        self.loaded["VCRUNTIME140.dll"] = self.vcruntime
        self.policy["targets"]["AMD64"]["python_loaded"]["VCRUNTIME140.dll"]["owner"] = "Windows System32"
        with self.assertRaisesRegex(RuntimeError, "owner or SHA-256 pin"):
            self.verify()

    def test_committed_pins_match_independently_reviewed_loaded_modules(self):
        here = Path(__file__).resolve().parent
        policy = json.loads((here / "windows-external-runtime.json").read_text(encoding="utf-8"))
        review = json.loads((here.parents[2] / "docs/sprint-503/evidence/python-installed-40214773709/review.json").read_text(encoding="utf-8"))
        self.assertEqual(set(policy["targets"]), {"AMD64", "ARM64"})
        for arch, key in (("AMD64", "amd64"), ("ARM64", "arm64")):
            loaded = policy["targets"][arch]["python_loaded"]
            self.assertEqual(set(loaded), set(probe.NAMES))
            self.assertEqual(loaded, review["windows"][key]["loaded_vc"])
            self.assertEqual(loaded["MSVCP140.dll"]["sha256"],
                             policy["targets"][arch]["system32"]["MSVCP140.dll"]["sha256"])
            self.assertNotEqual(loaded["VCRUNTIME140.dll"]["sha256"],
                                policy["targets"][arch]["system32"]["VCRUNTIME140.dll"]["sha256"])


if __name__ == "__main__":
    unittest.main()
