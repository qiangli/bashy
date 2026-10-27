import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("matrix_provision", Path(__file__).with_name("provision.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


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


if __name__ == "__main__":
    unittest.main()
