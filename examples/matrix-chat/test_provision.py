import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

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

    def test_setup_rerun_skips_existing_users_without_reset_or_admin_change(self):
        with tempfile.TemporaryDirectory() as temp:
            data = Path(temp) / "data"
            data.mkdir()
            binary = Path(temp) / "tuwunel"
            binary.touch()
            cfg = {"server_name": "localhost", "data_dir": str(data), "port": 8008,
                   "users": [{"localpart": "alice", "password_secret": "alice"},
                             {"localpart": "bob", "password_secret": "bob"}], "rooms": []}
            calls = []

            def fake_run(args, **kwargs):
                calls.append(args)
                if args[0] == str(binary) and args[-1] == "users list-users":
                    return subprocess.CompletedProcess(args, 0, "@alice:localhost\n@bob:localhost\n", "")
                if args[0] == str(binary) and args[1:3] == ["--test=smoke", "--execute"]:
                    return subprocess.CompletedProcess(args, 0, "", "")
                if args[:3] == ["bashy", "service", "status"]:
                    return subprocess.CompletedProcess(args, 0, "inactive\n", "")
                return subprocess.CompletedProcess(args, 0, "", "")

            with mock.patch.object(module, "installed_binary", return_value=str(binary)), \
                 mock.patch.object(module, "run"), \
                 mock.patch.object(module.subprocess, "run", side_effect=fake_run), \
                 mock.patch.object(module, "vault", side_effect=lambda ref: "secret-" + ref), \
                 mock.patch.object(module, "request", return_value={}), \
                 mock.patch.object(module, "provision"):
                module.setup(cfg)

            account_args = [arg for args in calls if args and args[0] == str(binary) for arg in args]
            self.assertNotIn("users reset-password alice", account_args)
            self.assertNotIn("users reset-password bob", account_args)
            self.assertNotIn("users make-user-admin @alice:localhost", account_args)

    def test_setup_fails_closed_on_user_enumeration_error(self):
        with tempfile.TemporaryDirectory() as temp:
            data = Path(temp) / "data"
            data.mkdir()
            binary = Path(temp) / "tuwunel"
            binary.touch()
            cfg = {"server_name": "localhost", "data_dir": str(data), "port": 8008,
                   "users": [{"localpart": "alice", "password_secret": "alice"}], "rooms": []}

            def fake_run(args, **kwargs):
                if args[:3] == ["bashy", "service", "status"]:
                    return subprocess.CompletedProcess(args, 0, "inactive\n", "")
                if args[0] == str(binary) and args[-1] == "users list-users":
                    return subprocess.CompletedProcess(args, 1, "", "enumeration failed")
                return subprocess.CompletedProcess(args, 0, "", "")

            with mock.patch.object(module, "installed_binary", return_value=str(binary)), \
                 mock.patch.object(module, "run"), \
                 mock.patch.object(module.subprocess, "run", side_effect=fake_run), \
                 mock.patch.object(module, "provision") as provision:
                with self.assertRaises(SystemExit):
                    module.setup(cfg)
                provision.assert_not_called()


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

    def test_missing_tool_lookup_does_not_install(self):
        result = subprocess.CompletedProcess([], 1, "", "not installed")
        with mock.patch.object(e2e.subprocess, "run", return_value=result) as run:
            with self.assertRaises(AssertionError):
                e2e.signed_installed_tuwunel("/fake/bashy")
        argv = run.call_args.args[0]
        self.assertEqual(argv, ["/fake/bashy", "tools", "show", "tuwunel", "--json"])
        self.assertNotIn("install", argv)


if __name__ == "__main__":
    unittest.main()
