import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys
import subprocess
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
sys.path.insert(0, str(here))
gate = load("cloudbox_gate", "cloudbox_gate.py")
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

    def __init__(self, exists=False, register=True, auth_ok=True, name_fails=False, logout_fails=False,
                 auth_drops=False, live=None):
        self._exists, self._register, self._auth_ok = exists, register, auth_ok
        self._name_fails, self._logout_fails = name_fails, logout_fails
        # auth_drops: the password change lands, then the transport fails.
        self._auth_drops, self._live = auth_drops, live
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
        if self._name_fails:
            raise shim.ShimError(502, "M_UNKNOWN", "display name update failed")

    def session(self, localpart):
        return "device-token"

    def logout(self, token):
        self.calls.append(("logout", token))
        if self._logout_fails:
            raise shim.ShimError(502, "M_UNKNOWN", "homeserver unreachable")

    def with_password_auth(self, localpart, token, path, body, password):
        self.calls.append(("uia", path, password))
        if self._auth_drops:
            if self._live is not None:
                self._live = body["new_password"]
            raise shim.ShimError(502, "M_UNKNOWN", "homeserver unreachable")
        return self._auth_ok

    def password_works(self, localpart, password):
        return None if self._live is None else password == self._live


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


    def test_display_name_failure_keeps_created_password(self):
        ref = "acct/%s" % LP
        vault, hs = Vault(), FakeHomeserver(name_fails=True)
        out = self.provisioner(hs, vault).handle({"localpart": LP, "action": "ensure", "display_name": "A"})
        self.assertTrue(out["created"])
        self.assertEqual(vault.values[ref], out["password"])

    def test_display_name_failure_keeps_completed_reset(self):
        ref = "acct/%s" % LP
        vault, hs = Vault({ref: "old"}), FakeHomeserver(exists=True, name_fails=True)
        out = self.provisioner(hs, vault).handle({"localpart": LP, "action": "reset", "display_name": "A"})
        self.assertEqual(vault.values[ref], out["password"])
        self.assertNotEqual(out["password"], "old")

    def test_display_name_failure_without_password_still_reports(self):
        hs = FakeHomeserver(exists=True, name_fails=True)
        with self.assertRaises(shim.ShimError) as ctx:
            self.provisioner(hs, Vault()).handle({"localpart": LP, "action": "ensure", "display_name": "A"})
        self.assertEqual(ctx.exception.status, 502)

    def test_logout_failure_does_not_mask_completed_reset(self):
        ref = "acct/%s" % LP
        vault, hs = Vault({ref: "old"}), FakeHomeserver(exists=True, logout_fails=True)
        out = self.provisioner(hs, vault).handle({"localpart": LP, "action": "reset"})
        self.assertEqual(vault.values[ref], out["password"])

    def test_logout_failure_does_not_mask_refused_reset(self):
        ref = "acct/%s" % LP
        vault = Vault({ref: "old"})
        hs = FakeHomeserver(exists=True, auth_ok=False, logout_fails=True)
        with self.assertRaises(shim.ShimError) as ctx:
            self.provisioner(hs, vault).handle({"localpart": LP, "action": "reset"})
        self.assertEqual(ctx.exception.status, 409)
        self.assertEqual(vault.values[ref], "old")

    def test_logout_failure_does_not_mask_deactivate(self):
        ref = "acct/%s" % LP
        vault = Vault({ref: "pw"})
        out = self.provisioner(FakeHomeserver(exists=True, logout_fails=True), vault).handle(
            {"localpart": LP, "action": "deactivate"})
        self.assertFalse(out["created"])
        self.assertEqual(vault.values, {})

    def test_reset_landed_before_transport_failure_is_completed(self):
        ref = "acct/%s" % LP
        vault, hs = Vault({ref: "old"}), FakeHomeserver(exists=True, auth_drops=True, live="old")
        out = self.provisioner(hs, vault).handle({"localpart": LP, "action": "reset"})
        self.assertEqual(vault.values[ref], out["password"])
        self.assertEqual(hs._live, out["password"])

    def test_reset_unknown_outcome_restores_and_reports(self):
        ref = "acct/%s" % LP
        vault = Vault({ref: "old"})
        hs = FakeHomeserver(exists=True, auth_drops=True)  # password_works cannot tell
        with self.assertRaises(shim.ShimError) as ctx:
            self.provisioner(hs, vault).handle({"localpart": LP, "action": "reset"})
        self.assertEqual(ctx.exception.status, 502)
        self.assertEqual(vault.values[ref], "old")


class GateTest(unittest.TestCase):
    def test_cleanup_uses_provisioner_ref_for_every_account(self):
        accounts = gate.Accounts()
        users = [accounts() for _ in range(3)]
        vault = gate.MemoryVault()
        prov = shim.Provisioner(FakeHomeserver(), vault, "gate/accounts/")
        for user in users:
            vault.set(prov.ref(user), "pw")
        vault.set("unrelated", "keep")
        self.assertEqual(gate.cleanup(prov, vault, accounts), [])
        self.assertEqual(vault.values, {"unrelated": "keep"})

    def test_cleanup_continues_past_a_failing_remove_and_reports_it(self):
        class Flaky(gate.MemoryVault):
            def remove(self, ref):
                if ref.endswith(first):
                    raise OSError("vault down")
                super().remove(ref)
        accounts = gate.Accounts()
        first, second = accounts(), accounts()
        vault = Flaky()
        prov = shim.Provisioner(FakeHomeserver(), vault, "gate/accounts")
        vault.set(prov.ref(first), "pw")
        vault.set(prov.ref(second), "pw")
        self.assertEqual(gate.cleanup(prov, vault, accounts), [prov.ref(first)])
        self.assertNotIn(prov.ref(second), vault.values)

    def tuwunel_meta(self, path, data, key=None):
        return {"cli": {"binary": str(path), "managed": {"assets": {
            key or gate.platform_key(): {"url": "https://example.invalid/tuwunel",
                                         "sha256": hashlib.sha256(data).hexdigest()}}}}}

    def test_tuwunel_override_must_be_the_verified_binary(self):
        with tempfile.TemporaryDirectory() as temp:
            admitted, other = Path(temp) / "tuwunel", Path(temp) / "other"
            admitted.write_bytes(b"signed")
            other.write_bytes(b"signed")
            link = Path(temp) / "link"
            os.symlink(admitted, link)
            meta = self.tuwunel_meta(admitted, b"signed")
            self.assertEqual(gate.tuwunel_binary("", meta), str(admitted))
            self.assertEqual(gate.tuwunel_binary(str(link), meta), str(admitted))
            for bad in (str(other), str(Path(temp) / "missing"), "/usr/bin/true"):
                with self.assertRaises(SystemExit):
                    gate.tuwunel_binary(bad, meta)

    def test_tuwunel_preflight_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            binary = Path(temp) / "tuwunel"
            binary.write_bytes(b"unsigned")
            cases = {
                "missing": self.tuwunel_meta(Path(temp) / "missing", b"signed"),
                "unsigned": self.tuwunel_meta(binary, b"signed"),
                "other platform": self.tuwunel_meta(binary, b"unsigned", "plan9/mips"),
                "relative": self.tuwunel_meta("tuwunel", b"unsigned"),
                "unexpanded": self.tuwunel_meta("$XDG/tuwunel", b"unsigned"),
                "no assets": {"cli": {"binary": str(binary)}},
                "empty": {},
            }
            for name, meta in cases.items():
                with self.subTest(name), self.assertRaises(SystemExit):
                    gate.tuwunel_binary("", meta)

    def test_tuwunel_home_path_is_expanded(self):
        with tempfile.TemporaryDirectory() as temp:
            binary = Path(temp) / "tools" / "tuwunel"
            binary.parent.mkdir()
            binary.write_bytes(b"signed")
            old = os.environ.get("HOME")
            os.environ["HOME"] = temp
            try:
                meta = self.tuwunel_meta("$HOME/tools/tuwunel", b"signed")
                self.assertEqual(gate.tuwunel_binary("", meta), str(binary))
            finally:
                if old is None:
                    os.environ.pop("HOME")
                else:
                    os.environ["HOME"] = old

    def test_tuwunel_preflight_never_installs(self):
        calls = []

        def run(argv, **kwargs):
            calls.append(list(argv))
            return subprocess.CompletedProcess(argv, 1, b"", b"")

        def install():
            raise AssertionError("gate preflight must not install tuwunel")
        real_run, real_install = gate.subprocess.run, gate.provision.installed_binary
        gate.subprocess.run, gate.provision.installed_binary = run, install
        try:
            with self.assertRaises(SystemExit):
                gate.tuwunel_binary("")
        finally:
            gate.subprocess.run, gate.provision.installed_binary = real_run, real_install
        self.assertEqual(calls, [["bashy", "tool", "show", "tuwunel", "--json"]])


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
