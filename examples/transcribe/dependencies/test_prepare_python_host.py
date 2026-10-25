"""Controlled checks for operator-host Python artifact preparation."""

import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
import unittest


HERE = Path(__file__).resolve().parent
SCRIPT = HERE / "prepare_python_host.sh"
POLICY = HERE / "external-python-runtime.json"
ADMISSION = HERE / "operator-python-admission.json"
POLICY_HASH = "bcd13fd6844b079f99f400b54bbe1c844d52ebb66579a6baeb35df5d42044a75"
ADMISSION_HASH = "bfedfa9d97509788c7532ad6675a334c2c7bbf84222b3c6a077cd0e0bc371d3f"


class PythonHostPolicyTests(unittest.TestCase):
    def test_all_six_targets_have_explicit_external_routes(self):
        policy = json.loads(POLICY.read_text(encoding="utf-8"))
        admission = json.loads(ADMISSION.read_text(encoding="utf-8"))
        windows = json.loads((HERE / "windows-external-runtime.json").read_text(encoding="utf-8"))
        self.assertEqual(set(policy["targets"]), {
            "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64",
            "windows/amd64", "windows/arm64",
        })
        self.assertEqual(set(admission["targets"]), set(policy["targets"]))
        for target, pin in policy["targets"].items():
            self.assertEqual(admission["targets"][target]["status"], "unadmitted")
            self.assertIsNone(admission["targets"][target]["installed_python_sha256"])
            self.assertEqual(len(pin["python_sha256"]), 64)
            if target.startswith("linux/"):
                self.assertIn("actions/python-versions", pin["archive_url"])
            elif target.startswith("darwin/"):
                self.assertTrue(pin["archive_url"].startswith("https://www.python.org/"))
            else:
                arch = target.split("/")[1].upper()
                companion = windows["targets"][arch]
                self.assertEqual(pin["python_sha256"], companion["python_sha256"])
                self.assertEqual(pin["python_version"], companion["python_version"])
                self.assertEqual(pin["archive_sha256"], companion["python_official_package"]["sha256"])
                self.assertEqual(pin["archive_url"], companion["python_official_package"]["url"])
                self.assertTrue(companion["python_official_installer"]["url"].startswith("https://www.python.org/"))
                self.assertEqual(set(companion["system32"]), {"MSVCP140.dll", "VCRUNTIME140.dll"})


@unittest.skipUnless(shutil.which("bashy") and platform.system() == "Darwin", "native macOS Bashy gate")
class MacHostPreparationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="s503-python-host ; [literal] ")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        deps = self.root / "dependencies"
        deps.mkdir()
        self.script = deps / "prepare_python_host.sh"
        self.policy = deps / "external-python-runtime.json"
        self.admission = deps / "operator-python-admission.json"
        self.policy.write_bytes(POLICY.read_bytes())
        self.admission.write_bytes(ADMISSION.read_bytes())
        self.script.write_text(SCRIPT.read_text(encoding="utf-8"), encoding="utf-8")
        self.env = dict(os.environ, BASHY_HINTS="off", TMPDIR=str(self.root))

    def repin_fixture_policy(self, update):
        policy = json.loads(self.policy.read_text(encoding="utf-8"))
        target = "darwin/arm64" if platform.machine().lower() in ("arm64", "aarch64") else "darwin/amd64"
        update(policy["targets"][target])
        self.policy.write_text(json.dumps(policy, sort_keys=True), encoding="utf-8")
        script = SCRIPT.read_text(encoding="utf-8").replace(
            POLICY_HASH, hashlib.sha256(self.policy.read_bytes()).hexdigest()
        )
        self.script.write_text(script, encoding="utf-8")

    def repin_fixture_admission(self, update):
        admission = json.loads(self.admission.read_text(encoding="utf-8"))
        target = "darwin/arm64" if platform.machine().lower() in ("arm64", "aarch64") else "darwin/amd64"
        update(admission["targets"][target])
        self.admission.write_text(json.dumps(admission, sort_keys=True), encoding="utf-8")
        script = self.script.read_text(encoding="utf-8").replace(
            ADMISSION_HASH, hashlib.sha256(self.admission.read_bytes()).hexdigest()
        )
        self.script.write_text(script, encoding="utf-8")

    def invoke(self, *args):
        return subprocess.run([shutil.which("bashy"), str(self.script), *map(str, args)],
                              text=True, capture_output=True, env=self.env, timeout=30)

    def test_plan_names_exact_official_package_and_unadmitted_status(self):
        result = self.invoke("--plan")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("installer_url=https://www.python.org/ftp/python/3.12.10/python-3.12.10-macos11.pkg", result.stdout)
        self.assertIn("status=unadmitted", result.stdout)
        self.assertIn("installer_review=pending_native_review", result.stdout)
        self.assertIn("ci_python_sha256=", result.stdout)

    def test_install_rejected_before_artifact_access_without_native_admission(self):
        result = self.invoke("--install", self.root / "absent.pkg")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unadmitted", result.stderr)
        self.assertNotIn("artifact path is absent", result.stderr)

    def test_existing_interpreter_rejected_before_execution_without_native_admission(self):
        marker = self.root / "executed"
        fake_python = self.root / "fake python"
        fake_python.write_text('#!/bin/sh\nprintf executed > "$EXECUTION_MARKER"\n', encoding="utf-8")
        fake_python.chmod(0o700)
        self.env["EXECUTION_MARKER"] = str(marker)
        result = self.invoke("--verify-existing", fake_python)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unadmitted", result.stderr)
        self.assertFalse(marker.exists())

    def test_changed_policy_rejected(self):
        self.policy.write_text(self.policy.read_text(encoding="utf-8") + " ", encoding="utf-8")
        result = self.invoke("--plan")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("policy bytes changed", result.stderr)

    def test_unsigned_package_rejected_after_matching_digest(self):
        package = self.root / "fake Python.org.pkg"
        package.write_bytes(b"unsigned fixture, no installation")
        digest = hashlib.sha256(package.read_bytes()).hexdigest()
        self.repin_fixture_policy(lambda pin: pin.update(archive_sha256=digest))
        result = self.invoke("--verify-artifact", package)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("installer signature is invalid", result.stderr)
        self.assertEqual(list(self.root.glob("bashy-python-installer.*")), [])

    def test_changed_input_during_staging_rejected_before_signature(self):
        package = self.root / "replaceable Python.org.pkg"
        package.write_bytes(b"original installer fixture")
        digest = hashlib.sha256(package.read_bytes()).hexdigest()
        self.repin_fixture_policy(lambda pin: pin.update(archive_sha256=digest))
        script = self.script.read_text(encoding="utf-8").replace(
            "stage_dir= staged_artifact=",
            "printf 'changed input' > \"$artifact\"\nstage_dir= staged_artifact=",
        )
        self.script.write_text(script, encoding="utf-8")
        result = self.invoke("--verify-artifact", package)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("staged installer differs from reviewed SHA-256", result.stderr)
        self.assertNotIn("signature is invalid", result.stderr)
        self.assertEqual(list(self.root.glob("bashy-python-installer.*")), [])

    def test_unavailable_private_staging_rejected(self):
        package = self.root / "fake Python.org.pkg"
        package.write_bytes(b"installer fixture")
        digest = hashlib.sha256(package.read_bytes()).hexdigest()
        self.repin_fixture_policy(lambda pin: pin.update(archive_sha256=digest))
        self.env["TMPDIR"] = str(self.root / "absent")
        result = self.invoke("--verify-artifact", package)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("private installer staging parent is absent", result.stderr)
        self.assertNotIn("signature is invalid", result.stderr)

    def test_changed_interpreter_never_executes(self):
        marker = self.root / "executed"
        fake_python = self.root / "fake python"
        fake_python.write_text('#!/bin/sh\nprintf executed > "$EXECUTION_MARKER"\n', encoding="utf-8")
        fake_python.chmod(0o700)
        digest = hashlib.sha256(fake_python.read_bytes()).hexdigest()
        self.repin_fixture_admission(lambda pin: pin.update(
            status="admitted", installer_review="approved", installed_python_sha256=digest
        ))
        fake_python.write_text(fake_python.read_text(encoding="utf-8") + "# changed\n", encoding="utf-8")
        env_before = self.env
        self.env = dict(self.env, EXECUTION_MARKER=str(marker))
        self.addCleanup(setattr, self, "env", env_before)
        result = self.invoke("--verify-existing", fake_python)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("installed interpreter differs", result.stderr)
        self.assertFalse(marker.exists())


if __name__ == "__main__":
    unittest.main()
