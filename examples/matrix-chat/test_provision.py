import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("matrix_provision", Path(__file__).with_name("provision.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

e2e_spec = importlib.util.spec_from_file_location("matrix_e2e", Path(__file__).with_name("e2e.py"))
e2e = importlib.util.module_from_spec(e2e_spec)
e2e_spec.loader.exec_module(e2e)


class ConfigTest(unittest.TestCase):
    def test_identity_cannot_change(self):
        with tempfile.TemporaryDirectory() as temp:
            cfg = {"server_name": "one.test", "data_dir": temp + "/data",
                   "users": [{"localpart": "alice", "password_secret": "test/alice"}]}
            path = Path(temp) / "config.json"
            path.write_text(json.dumps(cfg))
            one = module.config(path)
            module.identity(one, create=True)
            cfg["server_name"] = "two.test"
            path.write_text(json.dumps(cfg))
            with self.assertRaises(SystemExit):
                module.identity(module.config(path), create=True)

    def test_invalid_participant(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "config.json"
            path.write_text(json.dumps({"server_name": "one.test", "data_dir": temp + "/data",
                "users": [{"localpart": "alice", "password_secret": "test/alice"}],
                "rooms": [{"key": "r", "creator": "alice", "invitees": ["bob"]}]}))
            with self.assertRaises(SystemExit):
                module.config(path)


class SyntheticSecretAdapterTest(unittest.TestCase):
    def test_adapter_operations(self):
        real_bashy = e2e.find_genuine_bashy()
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            with e2e.synthetic_secret_adapter(root, real_bashy):
                # 1. gen
                p = subprocess.run(["bashy", "secret", "gen"], capture_output=True, text=True, check=True)
                token = p.stdout.strip()
                self.assertTrue(len(token) > 20)

                # 2. set & show
                ref = "test/user1/password"
                subprocess.run(["bashy", "secret", "set", ref], input="super-secret-pass", text=True, check=True)
                p = subprocess.run(["bashy", "secret", "show", ref], capture_output=True, text=True, check=True)
                self.assertEqual(p.stdout.strip(), "super-secret-pass")

                # 3. rm
                subprocess.run(["bashy", "secret", "rm", ref], check=True)
                p = subprocess.run(["bashy", "secret", "show", ref], capture_output=True, text=True)
                self.assertNotEqual(p.returncode, 0)

                # 4. dispatch other verbs to genuine bashy
                p = subprocess.run(["bashy", "--version"], capture_output=True, text=True)
                self.assertEqual(p.returncode, 0)
                self.assertIn("bashy", p.stdout.lower())

    def test_fail_closed_missing_tuwunel(self):
        with self.assertRaises(AssertionError):
            e2e.verify_signed_tuwunel_and_service("/nonexistent/bashy/binary")


if __name__ == "__main__":
    unittest.main()
