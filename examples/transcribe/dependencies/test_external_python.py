"""Pure preflight tests for the six external Python prerequisites."""

import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from dependencies import external_python as check


class ExternalPythonTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        root = Path(self.temp.name)
        self.python = root / "python.exe"
        self.python.write_bytes(b"reviewed test interpreter")
        self.policy_path = root / "external-python-runtime.json"
        self.policy = {
            "schema": "s503-external-python-runtime-v1",
            "targets": {target: {
                "python_version": "3.12.10",
                "python_sha256": hashlib.sha256(self.python.read_bytes()).hexdigest(),
                "archive_url": "https://example.test/python-test.zip",
                "archive_sha256": "1" * 64,
            } for target in check.TARGETS},
        }

    def verify(self, system="Linux", process="AMD64", native="AMD64", **kwargs):
        return check.verify_pins(
            self.policy, str(self.python), str(self.python), "3.12.10",
            system, process, native, **kwargs,
        )

    def test_all_six_targets_have_exact_pins(self):
        committed = json.loads(check.POLICY_PATH.read_text(encoding="utf-8"))
        self.assertEqual(set(committed["targets"]), check.TARGETS)
        for target, pin in committed["targets"].items():
            self.assertRegex(pin["python_version"], r"^3\.12\.(10|15)$")
            self.assertRegex(pin["python_sha256"], r"^[0-9a-f]{64}$")
            self.assertRegex(pin["archive_sha256"], r"^[0-9a-f]{64}$")
            self.assertTrue(pin["archive_url"].startswith("https://"), target)

    def test_target_selection_is_native_for_all_six(self):
        for system, machine, target in (
            ("Linux", "x86_64", "linux/amd64"), ("Linux", "aarch64", "linux/arm64"),
            ("Darwin", "x86_64", "darwin/amd64"), ("Darwin", "arm64", "darwin/arm64"),
            ("Windows", "AMD64", "windows/amd64"), ("Windows", "ARM64", "windows/arm64"),
        ):
            with self.subTest(target=target):
                _, actual = self.verify(system, machine, machine)
                self.assertEqual(actual, target)

    def test_missing_changed_or_wrong_architecture_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "explicitly name"):
            check.verify_pins(self.policy, "", str(self.python), "3.12.10", "Linux", "AMD64", "AMD64")
        with self.assertRaisesRegex(RuntimeError, "natively"):
            self.verify(process="AMD64", native="ARM64")
        with self.assertRaisesRegex(RuntimeError, "natively"):
            self.verify(system="Darwin", translated=True)
        with self.assertRaisesRegex(RuntimeError, "active interpreter"):
            other = self.python.parent / "other.exe"
            other.write_bytes(b"different")
            check.verify_pins(self.policy, str(other), str(self.python), "3.12.10", "Linux", "AMD64", "AMD64")
        with self.assertRaisesRegex(RuntimeError, "six-target pin set"):
            self.policy["targets"].pop("linux/amd64")
            self.verify()

    def test_changed_bytes_version_and_provenance_rejected(self):
        with self.assertRaisesRegex(RuntimeError, "version"):
            check.verify_pins(self.policy, str(self.python), str(self.python), "3.12.11", "Linux", "AMD64", "AMD64")
        self.python.write_bytes(b"changed interpreter")
        with self.assertRaisesRegex(RuntimeError, "SHA-256"):
            self.verify()
        self.policy["targets"]["linux/amd64"].pop("archive_sha256")
        with self.assertRaisesRegex(RuntimeError, "archive provenance"):
            self.verify()

    def test_windows_path_retains_system32_vc_verification(self):
        self.policy_path.write_text(json.dumps(self.policy))
        with patch.object(check, "POLICY_PATH", self.policy_path), patch.object(
            check.sys, "executable", str(self.python)
        ), patch.object(check.platform, "system", return_value="Windows"), patch.object(
            check.platform, "machine", return_value="AMD64"
        ), patch.object(check.platform, "python_version", return_value="3.12.10"), patch.object(
            check, "_windows_native_arch", return_value="amd64"
        ), patch.object(check, "_verify_windows_vc") as vc_check:
            self.assertEqual(check.verify_selected_python(str(self.python)), self.python.resolve())
            vc_check.assert_called_once()
            vc_check.side_effect = RuntimeError("System32 VC mismatch")
            with self.assertRaisesRegex(RuntimeError, "System32 VC mismatch"):
                check.verify_selected_python(str(self.python))

    def test_explicit_linux_source_receipt_requires_active_executable(self):
        from dependencies import setup_python_source
        self.policy_path.write_text(json.dumps(self.policy))
        receipt = {"executable": str(self.python), "status": "local_source_build_unadmitted"}
        with patch.dict(check.os.environ, {"BASHY_TRANSCRIBE_PYTHON_SOURCE_PREFIX": str(self.python.parent)}), patch.object(
            check, "POLICY_PATH", self.policy_path
        ), patch.object(check.sys, "executable", str(self.python)), patch.object(
            check.platform, "system", return_value="Linux"
        ), patch.object(check.platform, "machine", return_value="x86_64"), patch.object(
            check.platform, "uname", return_value=type("Uname", (), {"machine": "x86_64"})()
        ), patch.object(check.platform, "python_version", return_value=setup_python_source.VERSION), patch.object(
            setup_python_source, "verify_existing", return_value=receipt
        ) as verify:
            self.assertEqual(check.verify_selected_python(str(self.python)), self.python.resolve())
            verify.assert_called_once()
            other = self.python.parent / "other-python"
            other.write_bytes(b"other")
            with self.assertRaisesRegex(RuntimeError, "active Python differs"):
                check.verify_selected_python(str(other))


if __name__ == "__main__":
    unittest.main()
