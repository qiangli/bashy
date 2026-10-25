import hashlib
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import probe_runtime


class RuntimeProbeTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.cache = self.root / "toolcache"
        self.cache.mkdir()
        self.python = self.cache / "python"
        self.python.write_bytes(b"test-python")

    def test_ci_python_records_exact_bytes(self):
        with mock.patch.dict(os.environ, {"RUNNER_TOOL_CACHE": str(self.cache)}), \
             mock.patch.object(probe_runtime.sys, "executable", str(self.python)), \
             mock.patch.object(probe_runtime.platform, "system", return_value="Linux"), \
             mock.patch.object(probe_runtime.platform, "python_version", return_value="3.12.15"):
            result = probe_runtime.probe()
        self.assertEqual(result["python"]["sha256"], hashlib.sha256(b"test-python").hexdigest())
        self.assertEqual(result["python"]["version"], "3.12.15")
        self.assertEqual(result["windows_vc_runtime"], {})

    def test_host_python_outside_toolcache_is_rejected(self):
        other = self.root / "unreviewed-python"
        other.write_bytes(b"host-python")
        with mock.patch.dict(os.environ, {"RUNNER_TOOL_CACHE": str(self.cache)}), \
             mock.patch.object(probe_runtime.sys, "executable", str(other)):
            with self.assertRaisesRegex(RuntimeError, "not linked"):
                probe_runtime.probe()

    def test_macos_framework_binary_requires_toolcache_link(self):
        framework = self.root / "Library" / "Frameworks" / "python"
        framework.parent.mkdir(parents=True)
        framework.write_bytes(b"framework-python")
        selected = self.cache / "versions" / "3.12.10" / "arm64"
        (selected / "bin").mkdir(parents=True)
        launcher = selected / "bin" / "python3.12"
        launcher.symlink_to(framework)
        with mock.patch.dict(os.environ, {"RUNNER_TOOL_CACHE": str(self.cache),
                                      "pythonLocation": str(selected)}), \
             mock.patch.object(probe_runtime.sys, "executable", str(framework)), \
             mock.patch.object(probe_runtime.platform, "system", return_value="Darwin"), \
             mock.patch.object(probe_runtime.platform, "python_version", return_value="3.12.10"):
            result = probe_runtime.probe()
            self.assertEqual(result["python"]["tool_cache_link"], str(launcher))
            launcher.unlink()
            with self.assertRaisesRegex(RuntimeError, "not linked"):
                probe_runtime.probe()
        self.assertEqual(result["python"]["sha256"],
                         hashlib.sha256(b"framework-python").hexdigest())

    def test_windows_requires_both_system32_dlls_and_records_versions(self):
        system32 = self.root / "Windows" / "System32"
        system32.mkdir(parents=True)
        (system32 / "MSVCP140.dll").write_bytes(b"msvcp")
        environment = {"RUNNER_TOOL_CACHE": str(self.cache), "SystemRoot": str(system32.parent)}
        patches = [mock.patch.dict(os.environ, environment),
                   mock.patch.object(probe_runtime.sys, "executable", str(self.python)),
                   mock.patch.object(probe_runtime.platform, "system", return_value="Windows"),
                   mock.patch.object(probe_runtime.subprocess, "check_output", return_value="14.44.35211.0\n")]
        with patches[0], patches[1], patches[2], patches[3]:
            with self.assertRaises(FileNotFoundError):
                probe_runtime.probe()
            (system32 / "VCRUNTIME140.dll").write_bytes(b"vcruntime")
            result = probe_runtime.probe()
            with mock.patch.object(probe_runtime.subprocess, "check_output",
                                   return_value="0.0.0.0\n"):
                with self.assertRaisesRegex(RuntimeError, "unreadable VC runtime version"):
                    probe_runtime.probe()
        runtime = result["windows_vc_runtime"]
        self.assertEqual(set(runtime), set(probe_runtime.VC_DLLS))
        self.assertEqual(runtime["MSVCP140.dll"]["version"], "14.44.35211.0")
        self.assertEqual(runtime["VCRUNTIME140.dll"]["sha256"],
                         hashlib.sha256(b"vcruntime").hexdigest())


if __name__ == "__main__":
    unittest.main()
