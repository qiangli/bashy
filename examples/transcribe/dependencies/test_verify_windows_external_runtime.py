import hashlib
import json
from pathlib import Path
import tempfile
import unittest

import verify_windows_external_runtime as check


def digest(contents):
    return hashlib.sha256(contents).hexdigest()


class ExternalWindowsRuntimeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        root = Path(self.temp.name)
        self.python = root / "python.exe"
        self.python.write_bytes(b"reviewed python")
        self.system_root = root / "Windows"
        system32 = self.system_root / "System32"
        system32.mkdir(parents=True)
        self.msvcp = system32 / "MSVCP140.dll"
        self.vcruntime = system32 / "VCRUNTIME140.dll"
        self.msvcp.write_bytes(b"reviewed msvcp")
        self.vcruntime.write_bytes(b"reviewed vcruntime")
        self.policy = {"schema": "s503-windows-external-runtime-v1", "targets": {
            "AMD64": {"python_version": "3.12.10", "python_sha256": digest(b"reviewed python"),
                      "system32": {
                          "MSVCP140.dll": {"version": "14.51.36247.0", "sha256": digest(b"reviewed msvcp")},
                          "VCRUNTIME140.dll": {"version": "14.51.36247.0", "sha256": digest(b"reviewed vcruntime")}}}}}

    def verify(self, version="3.12.10", arch="AMD64", version_reader=None):
        reader = version_reader or (lambda _: "14.51.36247.0")
        return check.verify(self.policy, self.python, version, arch, self.system_root, reader)

    def test_exact_external_bytes_and_versions_pass(self):
        report = self.verify()
        self.assertEqual(report["python"]["sha256"], digest(b"reviewed python"))
        self.assertEqual(set(report["system32"]), set(check.VC_DLLS))

    def test_unpinned_or_wrong_python_fails_closed(self):
        for change in (lambda: self.policy["targets"]["AMD64"].pop("python_sha256"),
                       lambda: self.python.write_bytes(b"updated python")):
            with self.subTest(change=change):
                saved = self.policy["targets"]["AMD64"].copy()
                self.python.write_bytes(b"reviewed python")
                change()
                with self.assertRaisesRegex(RuntimeError, "Python.*SHA-256"):
                    self.verify()
                self.policy["targets"]["AMD64"] = saved
        with self.assertRaisesRegex(RuntimeError, "Python version"):
            self.verify(version="3.12.11")
        with self.assertRaisesRegex(RuntimeError, "architecture"):
            self.verify(arch="ARM64")

    def test_missing_or_changed_system32_runtime_fails_closed(self):
        self.vcruntime.unlink()
        with self.assertRaises(FileNotFoundError):
            self.verify()
        self.vcruntime.write_bytes(b"updated vcruntime")
        with self.assertRaisesRegex(RuntimeError, "VCRUNTIME140.dll SHA-256"):
            self.verify()
        self.vcruntime.write_bytes(b"reviewed vcruntime")
        with self.assertRaisesRegex(RuntimeError, "version"):
            self.verify(version_reader=lambda _: "14.52.00000.0")

    def test_committed_policy_has_exact_pins_for_both_windows_architectures(self):
        policy = json.loads(check.POLICY.read_text(encoding="utf-8"))
        self.assertEqual(set(policy["targets"]), {"AMD64", "ARM64"})
        for target in policy["targets"].values():
            self.assertEqual(target["python_version"], "3.12.10")
            self.assertTrue(check._sha_pin(target["python_sha256"]))
            self.assertEqual(set(target["system32"]), set(check.VC_DLLS))
            for pin in target["system32"].values():
                self.assertTrue(check._sha_pin(pin["sha256"]))
                self.assertEqual(pin["version"], "14.51.36247.0")


if __name__ == "__main__":
    unittest.main()
