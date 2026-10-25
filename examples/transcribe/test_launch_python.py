"""Pre-execution launcher checks with a controlled executable fixture."""

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
TARGETS = (
    "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64",
    "windows/amd64", "windows/arm64",
)


@unittest.skipUnless(shutil.which("bashy") and os.name != "nt", "Bashy and a POSIX fixture are required")
class LauncherTests(unittest.TestCase):
    def test_committed_policy_matches_launcher_pin(self):
        policy = HERE / "dependencies" / "external-python-runtime.json"
        launcher = (HERE / "launch_python.sh").read_text(encoding="utf-8")
        self.assertIn(hashlib.sha256(policy.read_bytes()).hexdigest(), launcher)

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="s503-launch ; [literal] ")
        self.addCleanup(self.temp.cleanup)
        root = Path(self.temp.name)
        (root / "dependencies").mkdir()
        self.launcher = root / "launch_python.sh"
        self.python = root / "fixture python"
        self.python.write_text(
            '#!/bin/sh\nprintf "%s\\n" "$@" > "$LAUNCH_MARKER"\n', encoding="utf-8"
        )
        self.python.chmod(0o700)
        (root / "source_youtube.py").write_text("# controlled adapter path\n", encoding="utf-8")
        self.request = root / "request ; $(literal).json"
        self.request.write_text("{}", encoding="utf-8")
        self.marker = root / "executed"
        self.policy = root / "dependencies" / "external-python-runtime.json"
        interpreter_hash = hashlib.sha256(self.python.read_bytes()).hexdigest()
        self.policy.write_text(json.dumps({
            "schema": "s503-external-python-runtime-v1",
            "targets": {target: {"python_sha256": interpreter_hash} for target in TARGETS},
        }), encoding="utf-8")
        source = (HERE / "launch_python.sh").read_text(encoding="utf-8")
        policy_hash = hashlib.sha256(self.policy.read_bytes()).hexdigest()
        source = source.replace(
            "bcd13fd6844b079f99f400b54bbe1c844d52ebb66579a6baeb35df5d42044a75",
            policy_hash,
        )
        self.launcher.write_text(source, encoding="utf-8")
        self.env = dict(os.environ, BASHY_TRANSCRIBE_PYTHON=str(self.python),
                        LAUNCH_MARKER=str(self.marker), BASHY_HINTS="off")
        self.env.pop("BASHY_TRANSCRIBE_PYTHON_SOURCE_PREFIX", None)
        self.env.pop("BASHY_TRANSCRIBE_PYTHON_SHA256", None)

    def invoke(self):
        return subprocess.run(
            [shutil.which("bashy"), str(self.launcher), str(self.request)],
            env=self.env, text=True, capture_output=True, timeout=30,
        )

    def test_verified_interpreter_runs_with_literal_request_argv(self):
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.marker.read_text(encoding="utf-8").splitlines(),
                         [str(self.launcher.parent.resolve() / "source_youtube.py"), str(self.request)])

    def test_changed_interpreter_never_executes(self):
        self.python.write_text(self.python.read_text(encoding="utf-8") + "# changed\n", encoding="utf-8")
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("SHA-256 differs", result.stderr)
        self.assertFalse(self.marker.exists())

    def test_changed_policy_never_executes(self):
        self.policy.write_text(self.policy.read_text(encoding="utf-8") + " ", encoding="utf-8")
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("policy bytes changed", result.stderr)
        self.assertFalse(self.marker.exists())

    def test_missing_python_never_executes(self):
        self.env.pop("BASHY_TRANSCRIBE_PYTHON")
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("BASHY_TRANSCRIBE_PYTHON is required", result.stderr)
        self.assertFalse(self.marker.exists())

    def test_explicit_host_hash_works_without_ci_policy(self):
        self.env["BASHY_TRANSCRIBE_PYTHON_SHA256"] = hashlib.sha256(self.python.read_bytes()).hexdigest()
        self.policy.unlink()
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.marker.read_text(encoding="utf-8").splitlines(),
                         [str(self.launcher.parent.resolve() / "source_youtube.py"), str(self.request)])

    def test_explicit_host_hash_drift_never_executes(self):
        self.env["BASHY_TRANSCRIBE_PYTHON_SHA256"] = hashlib.sha256(self.python.read_bytes()).hexdigest()
        self.python.write_text(self.python.read_text(encoding="utf-8") + "# changed\n", encoding="utf-8")
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("SHA-256 differs", result.stderr)
        self.assertFalse(self.marker.exists())

    def test_explicit_host_invalid_or_ambiguous_pin_never_executes(self):
        self.env["BASHY_TRANSCRIBE_PYTHON_SHA256"] = "not-a-digest"
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("hash is invalid", result.stderr)
        self.assertFalse(self.marker.exists())
        self.env["BASHY_TRANSCRIBE_PYTHON_SOURCE_PREFIX"] = str(self.launcher.parent)
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("not both", result.stderr)
        self.assertFalse(self.marker.exists())

    @unittest.skipUnless(platform.system() == "Darwin", "non-Linux source prefix rejection")
    def test_source_prefix_on_darwin_never_executes(self):
        self.env["BASHY_TRANSCRIBE_PYTHON_SOURCE_PREFIX"] = str(self.launcher.parent)
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("supported only on Linux", result.stderr)
        self.assertFalse(self.marker.exists())


if __name__ == "__main__":
    unittest.main()
