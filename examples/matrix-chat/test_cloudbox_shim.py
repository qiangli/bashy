import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

here = Path(__file__).resolve().parent


def load(name, file):
    spec = importlib.util.spec_from_file_location(name, here / file)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


shim = load("cloudbox_shim", "cloudbox_shim.py")
provision = load("matrix_provision", "provision.py")
LP = "cb_" + "0" * 32


class Vault:
    def __init__(self, values=None, fail_set=False):
        self.values = dict(values or {})
        self.fail_set = fail_set

    def get(self, ref):
        return self.values.get(ref)

    def set(self, ref, value):
        if self.fail_set:
            raise OSError("vault down")
        self.values[ref] = value

    def remove(self, ref):
        self.values.pop(ref, None)


class FakeHomeserver:
    """Records calls; behavior is set per test."""

    def __init__(self, exists=False, register=True, auth_ok=True):
        self._exists, self._register, self._auth_ok = exists, register, auth_ok
        self.calls = []

    def user_id(self, localpart):
        return "@%s:test" % localpart

    def exists(self, localpart):
        return self._exists

    def register(self, localpart, password):
        self.calls.append(("register", password))
        return self._register

    def set_display_name(self, localpart, name):
        self.calls.append(("displayname", name))

    def session(self, localpart):
        return "device-token"

    def logout(self, token):
        self.calls.append(("logout", token))

    def with_password_auth(self, localpart, token, path, body, password):
        self.calls.append(("uia", path, password))
        return self._auth_ok


class ProvisionerTest(unittest.TestCase):
    def provisioner(self, hs, vault):
        return shim.Provisioner(hs, vault, "acct")

    def test_rejects_unmapped_localparts_and_actions(self):
        p = self.provisioner(FakeHomeserver(), Vault())
        for request in ({"localpart": "alice", "action": "ensure"},
                        {"localpart": "@%s:test" % LP, "action": "ensure"},
                        {"localpart": LP + "0", "action": "ensure"},
                        {"localpart": LP, "action": "make-admin"},
                        {"localpart": LP, "action": "ensure", "display_name": 7}):
            with self.assertRaises(shim.ShimError) as ctx:
                p.handle(request)
            self.assertEqual(ctx.exception.status, 400)

    def test_create_stores_before_register_and_returns_password_once(self):
        vault, hs = Vault(), FakeHomeserver()
        out = self.provisioner(hs, vault).handle({"localpart": LP, "action": "ensure", "display_name": " A "})
        self.assertTrue(out["created"])
        self.assertEqual(vault.values["acct/%s" % LP], out["password"])
        self.assertIn(("displayname", "A"), hs.calls)
        hs._exists = True
        again = self.provisioner(hs, vault).handle({"localpart": LP, "action": "ensure"})
        self.assertEqual(again, {"user_id": "@%s:test" % LP, "created": False})

    def test_create_race_restores_previous_vault_value(self):
        ref = "acct/%s" % LP
        vault = Vault({ref: "previous"})
        out = self.provisioner(FakeHomeserver(register=False), vault).handle({"localpart": LP, "action": "ensure"})
        self.assertEqual(out["created"], False)
        self.assertNotIn("password", out)
        self.assertEqual(vault.values[ref], "previous")
        vault = Vault()
        self.provisioner(FakeHomeserver(register=False), vault).handle({"localpart": LP, "action": "ensure"})
        self.assertEqual(vault.values, {})

    def test_vault_outage_creates_nothing(self):
        hs = FakeHomeserver()
        with self.assertRaises(shim.ShimError) as ctx:
            self.provisioner(hs, Vault(fail_set=True)).handle({"localpart": LP, "action": "ensure"})
        self.assertEqual(ctx.exception.status, 503)
        self.assertFalse([c for c in hs.calls if c[0] == "register"])

    def test_reset_missing_account_creates(self):
        out = self.provisioner(FakeHomeserver(), Vault()).handle({"localpart": LP, "action": "reset"})
        self.assertTrue(out["created"] and out["password"])

    def test_reset_rotates_with_current_password(self):
        ref = "acct/%s" % LP
        vault, hs = Vault({ref: "old"}), FakeHomeserver(exists=True)
        out = self.provisioner(hs, vault).handle({"localpart": LP, "action": "reset"})
        self.assertFalse(out["created"])
        self.assertEqual(vault.values[ref], out["password"])
        self.assertIn(("uia", "/_matrix/client/v3/account/password", "old"), hs.calls)
        self.assertIn(("logout", "device-token"), hs.calls)

    def test_reset_with_stale_credential_restores_vault(self):
        ref = "acct/%s" % LP
        vault, hs = Vault({ref: "old"}), FakeHomeserver(exists=True, auth_ok=False)
        with self.assertRaises(shim.ShimError) as ctx:
            self.provisioner(hs, vault).handle({"localpart": LP, "action": "reset"})
        self.assertEqual(ctx.exception.status, 409)
        self.assertEqual(vault.values[ref], "old")
        self.assertIn(("logout", "device-token"), hs.calls)

    def test_reset_without_host_credential_is_conflict(self):
        with self.assertRaises(shim.ShimError) as ctx:
            self.provisioner(FakeHomeserver(exists=True), Vault()).handle({"localpart": LP, "action": "reset"})
        self.assertEqual(ctx.exception.status, 409)

    def test_deactivate(self):
        ref = "acct/%s" % LP
        with self.assertRaises(shim.ShimError) as ctx:
            self.provisioner(FakeHomeserver(), Vault()).handle({"localpart": LP, "action": "deactivate"})
        self.assertEqual(ctx.exception.status, 404)
        vault = Vault({ref: "pw"})
        out = self.provisioner(FakeHomeserver(exists=True), vault).handle({"localpart": LP, "action": "deactivate"})
        self.assertEqual(out, {"user_id": "@%s:test" % LP, "created": False})
        self.assertEqual(vault.values, {})


class ForwardTest(unittest.TestCase):
    def test_only_client_roots_are_forwarded(self):
        allowed = ["/_matrix/client/versions", "/_matrix/client/v3/sync?since=x&timeout=30000",
                   "/_matrix/media/v3/upload?filename=a.png", "/.well-known/matrix/client",
                   "/_matrix/client/v3/rooms/%21r%3Atest/send/m.room.message/1"]
        denied = ["/", "/_cloudbox/v1/account", "/_matrix/federation/v1/version",
                  "/_matrix/key/v2/server", "/_synapse/admin/v1/users", "/_tuwunel/oidc/jwks",
                  "/_matrix/client/../../_cloudbox/v1/account", "/_matrix/client/%2e%2e/x",
                  "/_matrix/client//x", "/_matrix/clientx", "/_matrix/client/a%5c..%5cb"]
        for path in allowed:
            self.assertTrue(shim.forwardable(path), path)
        for path in denied:
            self.assertFalse(shim.forwardable(path), path)

    def test_account_refs_fit_the_vault_name_limit(self):
        p = shim.Provisioner(FakeHomeserver(), Vault(), "a" * shim.MAX_PREFIX)
        self.assertEqual(len(p.ref(LP)), 64)
        with self.assertRaises(ValueError):
            shim.Provisioner(FakeHomeserver(), Vault(), "a" * (shim.MAX_PREFIX + 1))

    def test_upstream_must_be_loopback(self):
        with self.assertRaises(ValueError):
            shim.Homeserver("http://10.0.0.1:6167", "test", "t")


class UnitTest(unittest.TestCase):
    def config(self, temp, extra):
        cfg = {"server_name": "chat.example.test", "data_dir": temp + "/data",
               "users": [{"localpart": "alice", "password_secret": "matrix/alice"}]}
        cfg.update(extra)
        path = Path(temp) / "config.json"
        path.write_text(json.dumps(cfg))
        return provision.config(path)

    def test_cloudbox_section_defaults_and_validation(self):
        with tempfile.TemporaryDirectory() as temp:
            cfg = self.config(temp, {"cloudbox": {}})
            self.assertEqual(cfg["cloudbox"]["port"], 6168)
            for bad in ({"port": 6167}, {"tunnel_token_secret": "has space"}, {"unknown": 1},
                        {"account_secret_prefix": "x" * 29}):
                with self.assertRaises(SystemExit):
                    self.config(temp, {"cloudbox": bad})

    def test_unit_and_registration_hold_no_secret_values(self):
        with tempfile.TemporaryDirectory() as temp:
            cfg = self.config(temp, {"cloudbox": {}})
            unit = provision.shim_unit(cfg)
            self.assertEqual(unit["expose"]["app"], {"name": "matrix-chat", "port": "http", "require_login": False})
            self.assertEqual({s["ref"] for s in unit["secrets"]},
                             {"matrix-chat/cloudbox/tunnel-token", "matrix-chat/appservice/as-token"})
            self.assertEqual(unit["ports"][0]["bind"], "127.0.0.1")
            toml = provision.appservice_toml(cfg)
            self.assertIn('regex = "@cb_[0-9a-f]{32}:chat\\\\.example\\\\.test"', toml)
            self.assertIn("exclusive = true", toml)
            self.assertNotIn("token", toml)


if __name__ == "__main__":
    unittest.main()
