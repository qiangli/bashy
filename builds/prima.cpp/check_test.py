"""Bounded offline checks: wrong pin/dirty source/license cannot pass inspection."""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("prima_check", HERE / "check.py")
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


class AdmissionTest(unittest.TestCase):
    def test_candidate_is_explicitly_not_a_release(self):
        candidate = json.loads((HERE / "candidate.json").read_text())
        check.validate(candidate)
        for field, value in (("status", "approved"), ("blockers", [])):
            changed = copy.deepcopy(candidate)
            changed[field] = value
            with self.assertRaises(ValueError):
                check.validate(changed)
        result = subprocess.run(["python3", str(HERE / "check.py"), "--require-release"], capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn("release refused", result.stderr)
        self.assertFalse(json.loads(result.stdout)["build_permitted"])

    def test_exact_source_inspection(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            def git(*args):
                return check.git(root, *args)
            git("init", "-q")
            (root / "LICENSE").write_text("test license\n")
            git("add", "LICENSE")
            git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
            source = {"repository": "fixture", "commit": git("rev-parse", "HEAD"),
                      "license_path": "LICENSE", "license_sha256": hashlib.sha256(b"test license\n").hexdigest()}
            check.inspect(root, source)
            for change in ({"commit": "0" * 40}, {"license_sha256": "0" * 64}):
                with self.assertRaises(ValueError):
                    check.inspect(root, dict(source, **change))
            (root / "untracked").write_text("unexpected build input")
            with self.assertRaisesRegex(ValueError, "not pristine"):
                check.inspect(root, source)
            (root / "untracked").unlink()
            (root / "LICENSE").write_text("modified")
            with self.assertRaisesRegex(ValueError, "not pristine"):
                check.inspect(root, source)


if __name__ == "__main__":
    unittest.main()
