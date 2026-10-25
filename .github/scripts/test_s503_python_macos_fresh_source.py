"""Pure guard tests for the CI-only macOS fresh-prefix research lane."""

import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock


SCRIPT = Path(__file__).with_name("s503-python-macos-fresh-source.py")
SPEC = importlib.util.spec_from_file_location("s503_python_macos_fresh_source", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class FreshSourceGuards(unittest.TestCase):
    def test_source_verifier_pins_still_match(self):
        self.assertEqual(MODULE.load_setup().SOURCE_SHA256,
                         MODULE.PINS["Python-3.12.15.tar.xz"][1])

    def test_notice_copies_retain_exact_bytes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "source"
            evidence = Path(directory) / "evidence"
            root.mkdir()
            evidence.mkdir()
            (root / "LICENSE").write_bytes(b"PSF test notice\n")
            inventory = MODULE.notice_inventory(root, evidence, "source")
            self.assertTrue(inventory["all_copies_verified"])
            row = inventory["files"][0]
            self.assertEqual(row["sha256"], MODULE.sha256(evidence / row["retained_copy"]))

    def test_config_log_retains_full_diagnostic_when_bounded(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            log = root / "config.log"
            log.write_text("conftest link failed: missing SDK\n", encoding="utf-8")
            receipt = MODULE.retain_config_log(log, root)
            self.assertTrue(receipt["full_copy"])
            self.assertEqual((root / receipt["retained_copy"]).read_bytes(), log.read_bytes())
            self.assertEqual(receipt["sha256"], receipt["retained_copy_sha256"])

    def test_config_log_caps_large_diagnostic(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            log = root / "config.log"
            log.write_bytes(b"x" * (MODULE.MAX_CONFIG_LOG_COPY_BYTES + 1) + b"link failure")
            receipt = MODULE.retain_config_log(log, root)
            self.assertFalse(receipt["full_copy"])
            retained = (root / receipt["retained_copy"]).read_bytes()
            self.assertEqual(len(retained), MODULE.MAX_CONFIG_LOG_TAIL_BYTES)
            self.assertTrue(retained.endswith(b"link failure"))

    def test_sdk_is_checked_and_exported_to_restricted_build_env(self):
        with tempfile.TemporaryDirectory() as directory:
            sdk = Path(directory) / "MacOSX.sdk"
            stub = sdk / "usr/lib/libSystem.tbd"
            stub.parent.mkdir(parents=True)
            stub.write_bytes(b"native macOS linker stub")
            env = {"PATH": "/usr/bin:/bin"}
            with mock.patch.object(MODULE, "command", return_value=str(sdk) + "\n"):
                receipt = MODULE.macos_sdk(env)
            self.assertEqual(env["SDKROOT"], str(sdk.resolve(strict=True)))
            self.assertEqual(receipt["libsystem_stub_sha256"], MODULE.sha256(stub))

    def test_sdk_without_libsystem_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            sdk = Path(directory) / "MacOSX.sdk"
            sdk.mkdir()
            with mock.patch.object(MODULE, "command", return_value=str(sdk) + "\n"):
                with self.assertRaisesRegex(RuntimeError, "libSystem"):
                    MODULE.macos_sdk({"PATH": "/usr/bin:/bin"})

    def test_runtime_rejects_module_from_existing_framework(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            prefix = root / "prefix"
            executable = prefix / "bin/python3.12"
            executable.parent.mkdir(parents=True)
            executable.write_bytes(b"private test executable")
            outside = root / "existing-framework-ssl.so"
            outside.write_bytes(b"preexisting module")
            info = {"version": "3.12.15", "pointer_bits": 64,
                    "prefix": str(prefix), "base_prefix": str(prefix),
                    "executable": str(executable),
                    "executable_sha256": MODULE.sha256(executable),
                    "machine": "arm64", "stdlib_roundtrip": {"zlib": True},
                    "openssl_version": "OpenSSL 3.0.0",
                    "modules": {"_ssl": {"path": str(outside),
                                         "sha256": MODULE.sha256(outside)}}}
            with mock.patch.object(MODULE, "command", return_value=json.dumps(info)):
                with self.assertRaisesRegex(RuntimeError, "outside private prefix"):
                    MODULE.runtime(prefix, "darwin/arm64",
                                   {"bin/python3.12": {"sha256": MODULE.sha256(executable)}})

    def test_runner_guard_rejects_non_github_hosted_execution(self):
        with mock.patch.dict(os.environ, {"EXPECTED_TARGET": "darwin/arm64",
                                          "RUNNER_LABEL": "macos-15",
                                          "GITHUB_ACTIONS": "false",
                                          "RUNNER_ENVIRONMENT": "github-hosted"}):
            with mock.patch.object(MODULE.platform, "system", return_value="Darwin"), \
                 mock.patch.object(MODULE.platform, "machine", return_value="arm64"):
                with self.assertRaisesRegex(RuntimeError, "GitHub-hosted"):
                    MODULE.native_identity()


if __name__ == "__main__":
    unittest.main()
