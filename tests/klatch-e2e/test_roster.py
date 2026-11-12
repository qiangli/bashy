"""Offline checks of the roster journey helper run.py imports."""
import base64
import hashlib
import hmac as hmac_mod
import importlib.util
import json
import os
import tempfile
import time
import types
import unittest
from pathlib import Path
from unittest import mock

SPEC = importlib.util.spec_from_file_location("klatch_roster", Path(__file__).with_name("roster.py"))
r = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(r)

RUN_SPEC = importlib.util.spec_from_file_location("klatch_e2e_run", Path(__file__).with_name("run.py"))
runner = importlib.util.module_from_spec(RUN_SPEC)
RUN_SPEC.loader.exec_module(runner)

URL = "http://127.0.0.1:7100/"
SECRET = "sso-test-secret"

# SHA-256 DigestInfo prefix (independent of the module under test)
DIGESTINFO_SHA256 = bytes.fromhex("3031300d060960864801650304020105000420")


def make_args(**kw):
    base = dict(sso_secret=None)
    base.update(kw)
    return types.SimpleNamespace(**base)


class FakeJourney:
    def __init__(self, args, url, secret):
        self.args, self.url, self.secret = args, url, secret
        self.calls = 7
        FakeJourney.last = self

    def close(self):
        return {"status": "done", "detail": "no roster journey tree"}


def passing(name):
    return lambda sb, ctx: ({}, [f"{name}_ok"])


def all_passing(**over):
    funcs = {name: passing(name) for name in r.STEPS}
    funcs.update(over)
    return funcs


def boom(sb, ctx):
    raise r.Check("broken")


def blocked(sb, ctx):
    raise r.Blocked("not here")


class RosterCounts(unittest.TestCase):
    def test_roster_counts_reads_the_four_counters(self):
        self.assertEqual(r.roster_counts({"members": 2, "created": 2, "enabled": 0, "disabled": 0}),
                         {"members": 2, "created": 2, "enabled": 0, "disabled": 0})
        self.assertIsNone(r.roster_counts("not json"))
        self.assertIsNone(r.roster_counts(None))
        self.assertEqual(r.roster_counts({"members": 1})["created"], None)

    def test_assert_counts_accepts_exact(self):
        r.assert_counts("x", {"members": 2, "created": 2, "enabled": 0, "disabled": 0},
                        members=2, created=2, enabled=0, disabled=0)

    def test_assert_counts_rejects_mismatch(self):
        with self.assertRaises(r.Check) as cm:
            r.assert_counts("first roster push",
                            {"members": 2, "created": 1, "enabled": 0, "disabled": 0},
                            members=2, created=2, enabled=0, disabled=0)
        self.assertIn("created", str(cm.exception))
        self.assertIn("first roster push", str(cm.exception))

    def test_assert_counts_rejects_missing_counters(self):
        for bad in (None, {}, {"members": 2}, "oops"):
            with self.assertRaises(r.Check) as cm:
                r.assert_counts("repeat roster push", bad, members=2, created=0, enabled=0, disabled=0)
            self.assertIn("no counts", str(cm.exception))

class SsoHeaders(unittest.TestCase):
    def test_headers_match_the_server_hmac(self):
        headers = r.sso_headers("sec", "a@x.test", role="user", prefix="/p", ts=1700000000)
        want = hmac_mod.new(b"sec", b"a@x.test\nuser\n/p\n1700000000", hashlib.sha256).hexdigest()
        self.assertEqual(headers, {
            "Remote-User": "a@x.test", "Remote-Groups": "user", "X-Forwarded-Prefix": "/p",
            "X-Outpost-Identity-Ts": "1700000000", "X-Outpost-Identity-Sig": want})

    def test_headers_stamp_current_time_by_default(self):
        before = int(time.time())
        headers = r.sso_headers("sec", "a@x.test")
        after = int(time.time())
        self.assertTrue(before <= int(headers["X-Outpost-Identity-Ts"]) <= after)


class Classifiers(unittest.TestCase):
    def test_endpoint_probe_classification(self):
        self.assertIsNone(r.endpoint_present(401))
        reason = r.endpoint_present(404)
        self.assertIn("predates", reason)
        self.assertIn("1.2.7", reason)
        self.assertIn("--image", reason)
        self.assertIn("roster endpoint probe answered HTTP 500", r.endpoint_present(500))

    def test_sign_in_and_contact_answers(self):
        self.assertEqual(r.sign_in_answer(403), "refused")
        self.assertEqual(r.sign_in_answer(200), 200)
        self.assertEqual(r.contact_answer(405), "refused")
        self.assertEqual(r.contact_answer(200), 200)

    def test_leaked_reports_names_never_values(self):
        self.assertEqual(r.leaked("x hunter2 y", {"admin_password": "hunter2", "empty": ""}),
                         ["admin_password"])

    def test_resolve_secret_cli_env_generated(self):
        with mock.patch.dict(os.environ, {r.SECRET_ENV: "from-env"}):
            self.assertEqual(r.resolve_secret(make_args(sso_secret="cli")), ("cli", "cli"))
            self.assertEqual(r.resolve_secret(make_args()), ("from-env", "env"))
        with mock.patch.dict(os.environ, {}, clear=False):
            os.environ.pop(r.SECRET_ENV, None)
            secret, source = r.resolve_secret(make_args())
            self.assertEqual(source, "generated")
            self.assertTrue(secret)


class RsaSigning(unittest.TestCase):
    """The pure-stdlib RSA that builds signed external identities."""

    def test_keypair_and_signature_round_trip(self):
        n, e, d = r.rsa_keypair(1024)
        self.assertEqual(n.bit_length(), 1024)
        payload = b"identity payload"
        sig = r.sign_pkcs1v15_sha256(n, d, payload)
        em = pow(int.from_bytes(sig, "big"), e, n).to_bytes((n.bit_length() + 7) // 8, "big")
        self.assertEqual(em.lstrip(b"\x00"), DIGESTINFO_SHA256 + hashlib.sha256(payload).digest())

    def test_public_key_pem_is_spki_der(self):
        n, e = 0x00C8A3E3A7B2C5D1F0918273645ED0F1A2B3C4D5E6F708192A3B4C5D6E7F8091A2B3C4D5E6F708192A3B4C5D6E7F8091A2B3C4D5E6F708192A3B4C5D6E7F8091A2B3C4D5E6F708192A3B4C5D6E7F8091A2B3C4D5E6F708192A3B4C5D6E7F8091A2B3C4D5E6F708192A3B4C5D6E7F8091A2B3C4D5E6F708192A3B4C5D6E7F8091FF, 65537
        pem = r.public_key_pem(n, e)
        self.assertTrue(pem.startswith("-----BEGIN PUBLIC KEY-----\n"))
        self.assertTrue(pem.endswith("\n-----END PUBLIC KEY-----\n"))
        der = base64.b64decode("".join(pem.strip().splitlines()[1:-1]))

        def tlv(buf, i):
            tag = buf[i]
            ln = buf[i + 1]
            if ln & 0x80:
                width = ln & 0x7F
                ln = int.from_bytes(buf[i + 2:i + 2 + width], "big")
                i += 2 + width
            else:
                i += 2
            return tag, buf[i:i + ln], i + ln

        tag, body, i = tlv(der, 0)
        self.assertEqual(tag, 0x30)
        tag, alg, i = tlv(body, 0)
        self.assertEqual((tag, alg[:2]), (0x30, b"\x06\x09"))
        tag, bitstr, i = tlv(body, i)
        self.assertEqual((tag, bitstr[0]), (0x03, 0x00))
        tag, key, _ = tlv(bitstr, 1)
        self.assertEqual(tag, 0x30)
        tag, n_int, i = tlv(key, 0)
        tag, e_int, _ = tlv(key, i)
        self.assertEqual((tag, int.from_bytes(n_int, "big"), int.from_bytes(e_int, "big")),
                         (0x02, n, e))

    def test_identity_message_shape_and_guid(self):
        msg = r.build_identity_message(node="https://elsewhere.example.test", bits=1024)
        self.assertEqual(msg["keyType"], "RSA2048")
        self.assertEqual(msg["signatureType"], "PKCS1v15")
        pem = base64.b64decode(msg["publicKey"]).decode()
        payload = base64.b64decode(msg["message"])
        signed = json.loads(payload)
        identity = json.loads(signed["value"])
        self.assertEqual(identity["node"], "https://elsewhere.example.test")
        self.assertEqual(signed["messageType"], "identity")
        self.assertEqual(signed["guid"], hashlib.sha256(pem.encode()).hexdigest())
        n, e, d = None, None, None  # recovered from the PEM below
        der = base64.b64decode("".join(pem.strip().splitlines()[1:-1]))

        def tlv(buf, i):
            tag = buf[i]
            ln = buf[i + 1]
            if ln & 0x80:
                width = ln & 0x7F
                ln = int.from_bytes(buf[i + 2:i + 2 + width], "big")
                i += 2 + width
            else:
                i += 2
            return tag, buf[i:i + ln], i + ln

        _, body, _ = tlv(der, 0)
        _, _, i = tlv(body, 0)
        _, bitstr, _ = tlv(body, i)
        _, key, _ = tlv(bitstr, 1)
        _, n_int, i = tlv(key, 0)
        _, e_int, _ = tlv(key, i)
        n, e = int.from_bytes(n_int, "big"), int.from_bytes(e_int, "big")
        sig = base64.b64decode(msg["signature"])
        em = pow(int.from_bytes(sig, "big"), e, n).to_bytes((n.bit_length() + 7) // 8, "big")
        self.assertEqual(em.lstrip(b"\x00"), DIGESTINFO_SHA256 + hashlib.sha256(payload).digest())

    def test_identity_message_without_node_for_unknown_guid(self):
        msg = r.build_identity_message(node="", bits=1024)
        identity = json.loads(json.loads(base64.b64decode(msg["message"]))["value"])
        self.assertEqual(identity["node"], "")


class JourneySetup(unittest.TestCase):
    def _fake_provision(self, calls):
        client = types.SimpleNamespace(
            setup_admin=lambda pw: calls.append(("setup_admin", pw)),
            admin_session=lambda pw: calls.append(("admin_session", pw)) or "sess-tok",
            set_config=lambda sess, domain=None: calls.append(("set_config", sess, domain)))
        module = types.SimpleNamespace(Client=lambda base, resolve=None: client)
        return module, client

    def _journey(self):
        return r.Journey(make_args(), URL, SECRET)

    def test_claimed_store_is_blocked_before_anything(self):
        jr = self._journey()
        with mock.patch.object(jr, "admin_status", return_value=False), \
                mock.patch.object(r, "load_provision") as lp:
            with self.assertRaises(r.Blocked) as cm:
                jr.setup()
        lp.assert_not_called()
        self.assertIn("already claimed", str(cm.exception))

    def test_missing_endpoint_blocks_with_local_build_instructions(self):
        jr = self._journey()
        with mock.patch.object(jr, "admin_status", return_value=True), \
                mock.patch.object(jr, "http", return_value=(404, "404 page not found")), \
                mock.patch.object(r, "load_provision") as lp:
            with self.assertRaises(r.Blocked) as cm:
                jr.setup()
        lp.assert_not_called()
        self.assertIn("1.2.7", str(cm.exception))
        self.assertIn("oci build", str(cm.exception))

    def test_setup_probes_then_bootstraps_admin_and_empty_domain(self):
        jr = self._journey()
        calls = []
        module, client = self._fake_provision(calls)
        probe = mock.Mock(return_value=(401, "token not set"))

        def fake_http(method, path, query=None, body=None, headers=None, timeout=30):
            return probe(method, path, query, body, headers)

        with mock.patch.object(jr, "admin_status", return_value=True), \
                mock.patch.object(jr, "http", side_effect=fake_http), \
                mock.patch.object(r, "load_provision", return_value=module):
            session = jr.setup()
        self.assertEqual(session, "sess-tok")
        probe.assert_called_once_with("PUT", "/admin/sso/roster",
                                      {"token": "endpoint-probe"},
                                      {"owner": "probe@example.com", "emails": []}, None)
        self.assertEqual([c[0] for c in calls], ["setup_admin", "admin_session", "set_config"])
        self.assertEqual(calls[-1], ("set_config", "sess-tok", ""))
        self.assertEqual(jr.secrets["admin_session"], "sess-tok")
        self.assertIs(jr.client, client)

    def test_http_counts_calls_and_never_raises_on_errors(self):
        jr = self._journey()

        class Resp:
            status = 200

            def __enter__(self):
                return self

            def __exit__(self, *exc):
                return False

            def read(self):
                return b'{"ok": true}'

        import urllib.error
        err = urllib.error.HTTPError("u", 405, "nope", {}, None)
        err.read = lambda: b"405"
        with mock.patch.object(r.urllib.request, "urlopen", side_effect=[Resp(), err]):
            self.assertEqual(jr.http("POST", "/x", {"a": 1}, {"b": 2}), (200, {"ok": True}))
            # "405" is a valid JSON number: the body is parsed, never raised
            self.assertEqual(jr.http("POST", "/x"), (405, 405))
        self.assertEqual(jr.calls, 2)

    def test_sso_post_scans_the_answer_for_secrets(self):
        jr = self._journey()
        jr.secrets["sso_secret"] = "supersecretvalue"
        with mock.patch.object(jr, "http", return_value=(200, "supersecretvalue")):
            with self.assertRaises(r.Check) as cm:
                jr.sso_post("a@x.test")
        self.assertIn("sso_secret", str(cm.exception))
        self.assertNotIn("supersecretvalue", str(cm.exception))

    def test_sso_sign_in_redeems_the_one_time_code(self):
        jr = self._journey()
        answers = [(200, "abcd1234"), (200, {"guid": "g1", "appToken": "g1.tok"})]

        def fake_http(method, path, query=None, body=None, headers=None, timeout=30):
            got = answers.pop(0)
            if method == "PUT":
                self.assertEqual(path, "/account/access")
                self.assertEqual(query["token"], "abcd1234")
                self.assertEqual(query["appName"], r.SSO_APP)
            return got

        with mock.patch.object(jr, "http", side_effect=fake_http):
            login = jr.sso_sign_in("a@x.test")
        self.assertEqual(login["guid"], "g1")
        self.assertEqual(jr.token["a@x.test"], "g1.tok")

    def test_sso_sign_in_fails_closed_on_bad_status(self):
        jr = self._journey()
        with mock.patch.object(jr, "http", return_value=(403, "not on roster")):
            with self.assertRaises(r.Check):
                jr.sso_sign_in("a@x.test")
        jr2 = self._journey()
        with mock.patch.object(jr2, "http", side_effect=[(200, "code"), (401, "nope")]):
            with self.assertRaises(r.Check):
                jr2.sso_sign_in("a@x.test")


class Steps(unittest.TestCase):
    def ctx(self):
        return {"owner": "owner@e2e.example.test", "member": "member@e2e.example.test",
                "stranger": "stranger@e2e.example.test"}

    def test_step_push_asserts_both_count_sets(self):
        first = {"members": 2, "created": 2, "enabled": 0, "disabled": 0}
        repeat = {"members": 2, "created": 0, "enabled": 0, "disabled": 0}
        answers = [dict(first), dict(repeat)]
        sb = types.SimpleNamespace(push_roster=lambda o, e: answers.pop(0))
        detail, checks = r.step_push(sb, self.ctx())
        self.assertEqual(checks, ["first_push_counts", "repeat_push_noop"])
        self.assertEqual(detail, {"first": first, "repeat": repeat})
        wrong = types.SimpleNamespace(push_roster=lambda o, e: {"members": 9})
        with self.assertRaises(r.Check):
            r.step_push(wrong, self.ctx())

    def test_step_signin_asserts_guids_and_refusals(self):
        sb = types.SimpleNamespace(
            secret=SECRET,
            sso_sign_in=lambda email: {"guid": f"g-{email}", "appToken": "t"},
            sso_post=lambda email, headers=None: (403, "not on roster")
            if email != "owner@e2e.example.test" else (401, "no identity"))
        detail, checks = r.step_signin(sb, self.ctx())
        self.assertEqual(checks, ["owner_signin", "member_signin", "guids_distinct",
                                  "unlisted_refused", "unsigned_refused"])
        self.assertEqual(detail, {"stranger": "stranger@e2e.example.test"})

        same = types.SimpleNamespace(
            secret=SECRET, sso_sign_in=lambda email: {"guid": "one", "appToken": "t"},
            sso_post=lambda email, headers=None: (403, "x"))
        with self.assertRaises(r.Check):
            r.step_signin(same, self.ctx())

        admitted = types.SimpleNamespace(
            secret=SECRET, sso_sign_in=lambda email: {"guid": f"g-{email}", "appToken": "t"},
            sso_post=lambda email, headers=None: (200, "tok"))
        with self.assertRaises(r.Check) as cm:
            r.step_signin(admitted, self.ctx())
        self.assertIn("unlisted", str(cm.exception))

    def test_step_team_ownership_and_mesh(self):
        mod = types.SimpleNamespace(
            channel_subject=lambda c: c.get("subject"),
            card_status=lambda c: c.get("status"),
            card_guid=lambda c: c.get("guid"))
        team = [{"id": "t1", "subject": "Team"}]
        sb = types.SimpleNamespace(
            mod=mod,
            guid={"owner@e2e.example.test": "go", "member@e2e.example.test": "gm"},
            channels=lambda email: team if email.startswith("owner") else [],
            cards=lambda email: [{"guid": "gm" if email.startswith("owner") else "go",
                                  "status": "connected"}],
            card_for=None)
        sb.card_for = lambda email, guid: next(
            (c for c in sb.cards(email) if c["guid"] == guid), None)
        detail, checks = r.step_team(sb, self.ctx())
        self.assertEqual(detail, {"teamChannelId": "t1"})
        self.assertEqual(checks, ["team_owned_by_roster_owner", "member_not_team_owner",
                                  "members_meshed"])

        member_owns = types.SimpleNamespace(
            mod=mod, guid={"owner@e2e.example.test": "go", "member@e2e.example.test": "gm"},
            channels=lambda email: team, cards=lambda email: [], card_for=lambda e, g: None)
        with self.assertRaises(r.Check) as cm:
            r.step_team(member_owns, self.ctx())
        self.assertIn("belongs to the roster owner", str(cm.exception))

        unmeshed = types.SimpleNamespace(
            mod=mod, guid={"owner@e2e.example.test": "go", "member@e2e.example.test": "gm"},
            channels=lambda email: team if email.startswith("owner") else [],
            cards=lambda email: [{"guid": "gm", "status": "confirmed"}],
            card_for=lambda email, guid: {"guid": "gm", "status": "confirmed"})
        with self.assertRaises(r.Check) as cm:
            r.step_team(unmeshed, self.ctx())
        self.assertIn("mesh", str(cm.exception))

    def test_step_remove_asserts_counts_contacts_signin_and_reenable(self):
        removal = {"members": 1, "created": 0, "enabled": 0, "disabled": 1}
        restore = {"members": 2, "created": 0, "enabled": 1, "disabled": 0}
        pushes = [removal, restore]
        sb = types.SimpleNamespace(
            guid={"member@e2e.example.test": "gm"},
            push_roster=lambda o, e: pushes.pop(0),
            card_for=lambda email, guid: None,
            sso_post=lambda email, headers=None: (403, "not on roster"),
            sso_sign_in=lambda email: {"guid": "gm", "appToken": "t2"})
        detail, checks = r.step_remove(sb, self.ctx())
        self.assertEqual(checks, ["removal_counts", "removed_contacts_dropped",
                                  "removed_signin_refused", "disabled_proven_by_reenable",
                                  "restored_signin_works"])
        self.assertEqual(detail["removal"]["disabled"], 1)
        self.assertEqual(detail["restore"]["enabled"], 1)

        still_there = types.SimpleNamespace(
            guid={"member@e2e.example.test": "gm"},
            push_roster=lambda o, e: dict(removal) if not e else dict(restore),
            card_for=lambda email, guid: {"guid": "gm"},
            sso_post=lambda email, headers=None: (403, "x"),
            sso_sign_in=lambda email: {"guid": "g", "appToken": "t"})
        with self.assertRaises(r.Check) as cm:
            r.step_remove(still_there, self.ctx())
        self.assertIn("still in the owner's contacts", str(cm.exception))

        reactivated = types.SimpleNamespace(
            guid={"member@e2e.example.test": "gm"},
            push_roster=lambda o, e: dict(removal) if not e else dict(restore),
            card_for=lambda email, guid: None,
            sso_post=lambda email, headers=None: (200, "tok"),
            sso_sign_in=lambda email: {"guid": "g", "appToken": "t"})
        with self.assertRaises(r.Check) as cm:
            r.step_remove(reactivated, self.ctx())
        self.assertIn("removed member sign-in", str(cm.exception))

    def test_step_remove_rejects_wrong_restore_counts(self):
        removal = {"members": 1, "created": 0, "enabled": 0, "disabled": 1}
        wrong = {"members": 2, "created": 0, "enabled": 0, "disabled": 0}
        sb = types.SimpleNamespace(
            guid={"member@e2e.example.test": "gm"},
            push_roster=lambda o, e: dict(wrong) if not e else dict(removal),
            card_for=lambda email, guid: None,
            sso_post=lambda email, headers=None: (403, "x"),
            sso_sign_in=lambda email: {"guid": "g", "appToken": "t"})
        with self.assertRaises(r.Check) as cm:
            r.step_remove(sb, self.ctx())
        self.assertIn("member removal", str(cm.exception))

        no_reenable = types.SimpleNamespace(
            guid={"member@e2e.example.test": "gm"},
            push_roster=lambda o, e: dict(removal) if not e else dict(wrong),
            card_for=lambda email, guid: None,
            sso_post=lambda email, headers=None: (403, "x"),
            sso_sign_in=lambda email: {"guid": "g", "appToken": "t"})
        with self.assertRaises(r.Check) as cm:
            r.step_remove(no_reenable, self.ctx())
        self.assertIn("member restore", str(cm.exception))

    def test_step_federation_refuses_external_and_unknown(self):
        sb = types.SimpleNamespace(add_card=lambda email, msg: (405, "no"))
        with mock.patch.object(r, "build_identity_message",
                               side_effect=lambda node, **kw: {"node": node}):
            detail, checks = r.step_federation(sb, self.ctx())
        self.assertEqual(checks, ["external_node_refused", "unknown_guid_refused"])
        self.assertEqual(detail, {"node": "https://elsewhere.example.test"})

        accepted = types.SimpleNamespace(add_card=lambda email, msg: (200, {"id": "c"}))
        with mock.patch.object(r, "build_identity_message",
                               side_effect=lambda node, **kw: {"node": node}):
            with self.assertRaises(r.Check) as cm:
                r.step_federation(accepted, self.ctx())
        self.assertIn("external-node contact", str(cm.exception))


class JourneyDecisions(unittest.TestCase):
    def test_all_steps_pass(self):
        with mock.patch.dict(os.environ, {}, clear=False):
            os.environ.pop(r.SECRET_ENV, None)
            out = r.run_journey(make_args(), URL, SECRET, all_passing(), FakeJourney)
        self.assertEqual(r.summarize(out["steps"]), "PASS")
        self.assertEqual(out["checks_passed"], len(r.STEPS))
        self.assertEqual(out["http_calls"], 7)
        self.assertEqual(out["journey_teardown"]["status"], "done")
        self.assertEqual(len(out["accounts"]), 2)
        self.assertTrue(all("@" in a for a in out["accounts"]))

    def test_failure_blocks_every_later_step(self):
        out = r.run_journey(make_args(), URL, SECRET, all_passing(signin=boom), FakeJourney)
        s = out["steps"]
        self.assertEqual(r.summarize(s), "FAIL")
        self.assertEqual(s["signin"]["status"], "fail")
        self.assertEqual(s["team"]["status"], "blocked")
        self.assertEqual(s["federation"]["reason"], "skipped: signin failed")

    def test_blocked_setup_blocks_the_rest_with_the_reason(self):
        out = r.run_journey(make_args(), URL, SECRET, all_passing(setup=blocked), FakeJourney)
        s = out["steps"]
        self.assertEqual(r.summarize(s), "BLOCKED")
        self.assertEqual(s["setup"]["status"], "blocked")
        self.assertEqual(s["push"]["reason"], "skipped: setup blocked")

    def test_journey_setup_failure_is_fail(self):
        def factory(args, url, secret):
            raise OSError("no journey")
        out = r.run_journey(make_args(), URL, SECRET, all_passing(), factory)
        self.assertEqual(r.summarize(out["steps"]), "FAIL")
        self.assertEqual(out["journey_teardown"]["status"], "skipped")
        self.assertEqual(out["http_calls"], 0)

    def test_record_carries_no_secret(self):
        out = r.run_journey(make_args(), URL, SECRET, all_passing(), FakeJourney)
        self.assertNotIn(SECRET, json.dumps(out, default=str))

    def test_summarize_pending_is_not_a_pass(self):
        steps = {name: {"status": "pass"} for name in r.STEPS}
        steps["remove"] = {"status": "pending"}
        self.assertEqual(r.summarize(steps), "BLOCKED")


def roster_args(topology="none", url=None, evidence=None, image=None):
    args = types.SimpleNamespace(topology=topology, url=url, image=image, evidence=evidence,
                                 topic="General", expect_topics=["Team"])
    args.suite = "roster"
    return args


class RosterSuite(unittest.TestCase):
    """--suite roster runs the journey inside this runner's container lifecycle."""

    def hooks(self, log, journey=None, start=None):
        def default_start(topology, url, image, env=None, **kw):
            log.append(("start", topology, url, env))
            return {"container": "klatch-e2e-test", "image": "localhost/klatch-e2e:test"}

        def default_journey(args, url, secret):
            log.append(("journey", url, secret))
            return {"steps": {n: {"status": "pass", "checks": [n]} for n in runner.roster.STEPS},
                    "checks_passed": len(runner.roster.STEPS), "skipped_checks": [],
                    "http_calls": 11, "accounts": ["owner@x", "member@x"],
                    "journey_teardown": {"status": "done", "detail": "no roster journey tree"}}

        def stop(server):
            log.append(("stop", server["container"]))
            return {"status": "done", "detail": f"removed container {server['container']}"}

        return {"start_server": start or default_start, "stop_server": stop,
                "roster_journey": journey or default_journey,
                "candidate_info": lambda: {"bashy_version": "v", "bashy_sha": "abc1234"}}

    def test_secret_flows_to_container_and_journey(self):
        log = []
        args = roster_args()
        args.sso_secret = "unit-secret"
        rec = runner.run(args, self.hooks(log))
        self.assertEqual([e[0] for e in log], ["start", "journey", "stop"])
        self.assertEqual(log[0], ("start", "none", runner.LOCAL_URL, {"KLATCH_SSO_SECRET": "unit-secret"}))
        self.assertEqual(log[1], ("journey", runner.LOCAL_URL, "unit-secret"))
        self.assertEqual(rec["result"], "PASS")
        self.assertEqual(rec["suite"], "roster")
        self.assertEqual(rec["candidate"]["sso_secret_source"], "cli")
        self.assertNotIn("unit-secret", json.dumps(rec, default=str))

    def test_container_removed_even_when_the_journey_crashes(self):
        log = []

        def crash(args, url, secret):
            log.append(("journey", url))
            raise RuntimeError("roster endpoint exploded")
        rec = runner.run(roster_args(), self.hooks(log, journey=crash))
        self.assertEqual([e[0] for e in log], ["start", "journey", "stop"])
        self.assertEqual(rec["result"], "FAIL")
        self.assertIn("roster journey crashed", rec["steps"]["setup"]["reason"])

    def test_image_without_endpoint_is_blocked_and_still_torn_down(self):
        log = []

        def blocked_journey(args, url, secret):
            log.append(("journey", url))
            steps = {n: {"status": "pass"} for n in runner.roster.STEPS}
            steps["setup"] = {"status": "blocked",
                              "reason": runner.roster.ENDPOINT_BLOCKED}
            for later in runner.roster.STEPS[1:]:
                steps[later] = {"status": "blocked", "reason": "skipped: setup blocked"}
            return {"steps": steps, "checks_passed": 0, "skipped_checks": [], "http_calls": 1,
                    "accounts": [], "journey_teardown": {"status": "done", "detail": "x"}}
        rec = runner.run(roster_args(), self.hooks(log, journey=blocked_journey))
        self.assertEqual(rec["result"], "BLOCKED")
        self.assertIn("1.2.7", rec["steps"]["setup"]["reason"])
        self.assertEqual(rec["server_stop"]["status"], "done")
        self.assertEqual(rec["teardown"]["status"], "done")

    def test_oci_unavailable_is_blocked_without_journey(self):
        log = []

        def no_oci(topology, url, image, env=None, **kw):
            raise runner.Blocked("bashy oci unavailable: engine down")
        rec = runner.run(roster_args(), self.hooks(log, start=no_oci))
        self.assertEqual(log, [])
        self.assertEqual(rec["result"], "BLOCKED")
        self.assertEqual(rec["server"], {"status": "not-started"})
        self.assertEqual(rec["server_stop"]["status"], "skipped")

    def test_start_crash_is_fail_without_journey(self):
        log = []

        def crash(topology, url, image, env=None, **kw):
            raise RuntimeError("not healthy")
        rec = runner.run(roster_args(), self.hooks(log, start=crash))
        self.assertEqual(rec["result"], "FAIL")
        self.assertIn("klatch container start failed", rec["steps"]["setup"]["reason"])

    def test_non_none_topology_is_blocked_and_never_touches_a_server(self):
        for topology in ("outpost", "satellite", "both"):
            log = []
            rec = runner.run(roster_args(topology, url="https://live.example/"), self.hooks(log))
            self.assertEqual(log, [], topology)
            self.assertEqual(rec["result"], "BLOCKED")
            self.assertIn("disposable container", rec["steps"]["setup"]["reason"])

    def test_evidence_file_written(self):
        with tempfile.TemporaryDirectory() as d:
            ev = Path(d) / "ev.json"
            rec = runner.run(roster_args(evidence=str(ev)), self.hooks([]))
            on_disk = json.loads(ev.read_text())
        self.assertEqual(on_disk["suite"], "roster")
        self.assertEqual(on_disk["result"], "PASS")
        self.assertEqual(rec["evidence_path"], str(ev))

    def test_default_journey_is_the_imported_roster_helper(self):
        captured = {}

        def start(topology, url, image, env=None, **kw):
            return {"container": "c", "image": "i"}
        hooks = self.hooks([], start=start)
        del hooks["roster_journey"]
        with mock.patch.object(runner.roster, "run_journey",
                               side_effect=lambda a, u, s: captured.update(url=u, secret=s)
                               or {"steps": {}, "checks_passed": 0}):
            runner.run(roster_args(), hooks)
        self.assertEqual(captured["url"], runner.LOCAL_URL)
        self.assertTrue(captured["secret"])

    def test_parser_suite_and_roster_options(self):
        args = runner.build_parser().parse_args(
            ["--suite", "roster", "--sso-secret", "s3", "--image", "localhost/klatch-e2e:x"])
        self.assertEqual((args.suite, args.sso_secret), ("roster", "s3"))
        self.assertEqual(runner.build_parser().parse_args([]).suite, "browser")
        with self.assertRaises(SystemExit):
            runner.build_parser().parse_args(["--suite", "nope"])

    def test_start_server_passes_env_to_oci_run(self):
        calls = []
        images = json.dumps([{"Names": ["localhost/klatch:latest"]}])
        ok = types.SimpleNamespace(returncode=0, stdout="deadbeef", stderr="")

        def fake_run(argv, **kwargs):
            calls.append(argv)
            if "images" in argv:
                return types.SimpleNamespace(returncode=0, stdout=images, stderr="")
            if "rm" in argv:
                return types.SimpleNamespace(returncode=0, stdout="", stderr="")
            return ok

        with mock.patch.object(runner, "url_answers", return_value=False), \
                mock.patch.object(runner.subprocess, "run", side_effect=fake_run), \
                mock.patch.object(runner, "wait_for_health", return_value=True):
            server = runner.default_start_server(
                "none", runner.LOCAL_URL, None, env={"KLATCH_SSO_SECRET": "v"})
        run_argv = next(argv for argv in calls if "run" in argv)
        self.assertIn("-e", run_argv)
        self.assertEqual(run_argv[run_argv.index("-e") + 1], "KLATCH_SSO_SECRET=v")
        self.assertEqual(run_argv[-1], "localhost/klatch:latest")
        self.assertEqual(server["container"], run_argv[run_argv.index("--name") + 1])


class FakeKlatch:
    """An in-process Klatch faithful to the roster contract (klatch 5334662c).

    Implements exactly the endpoints the roster journey drives, with the
    reconciliation semantics of reconcileSSORoster / SetSSOAccess /
    AddCard: roster counts, HMAC-verified sign-in, one-time codes, the
    Team channel on the roster owner, the contact mesh and its removal,
    and the no-domain contact refusals.
    """

    def __init__(self, secret, with_roster=True):
        self.secret = secret
        self.with_roster = with_roster
        self.admin_password = None
        self.admin_session = "SESSION"
        self.configured = False
        self.roster = None            # {email: True} or None before first push
        self.accounts = {}            # email -> {"guid", "disabled", "tokens": []}
        self.codes = {}               # one-time code -> email
        self.team = None              # {"id", "owner": email}
        self.cards = {}               # email -> [{guid, status}]
        self.pushes = []

    def _guid(self, email):
        # the real server derives the guid from the key fingerprint: hex, no dots
        return hashlib.sha256(email.encode()).hexdigest()

    # --- roster reconciliation (mirrors reconcileSSORoster) ---
    def push(self, owner, emails):
        self.pushes.append((owner, list(emails)))
        wanted = {owner} | set(emails)
        result = {"members": len(wanted), "created": 0, "enabled": 0, "disabled": 0}
        for email in wanted:
            account = self.accounts.get(email)
            if account is None:
                self.accounts[email] = {"guid": self._guid(email), "disabled": False, "tokens": []}
                result["created"] += 1
            elif account["disabled"]:
                account["disabled"] = False
                result["enabled"] += 1
        removed = [e for e, a in self.accounts.items() if e not in wanted]
        for email in removed:
            if not self.accounts[email]["disabled"]:
                self.accounts[email]["disabled"] = True
                result["disabled"] += 1
            self.accounts[email]["tokens"] = []
            self.cards.pop(email, None)
        self.roster = set(wanted)
        self.team = {"id": "team-channel", "owner": owner}
        live = [e for e in sorted(wanted) if not self.accounts[e]["disabled"]]
        for email in live:  # the mesh
            self.cards[email] = [{"guid": self.accounts[o]["guid"], "status": "connected"}
                                 for o in live if o != email]
        return result

    # --- sign-in (mirrors SetSSOAccess) ---
    def verify(self, headers, now):
        stamp = headers.get("X-Outpost-Identity-Ts", "")
        try:
            ts = int(stamp)
        except ValueError:
            return None
        if abs(now - ts) > 60:
            return None
        mac = hmac_mod.new(self.secret.encode(),
                           "\n".join([headers.get("Remote-User", ""),
                                      headers.get("Remote-Groups", ""),
                                      headers.get("X-Forwarded-Prefix", ""),
                                      stamp]).encode(), hashlib.sha256).hexdigest()
        if not hmac_mod.compare_digest(mac, headers.get("X-Outpost-Identity-Sig", "")):
            return None
        if not headers.get("X-Forwarded-Prefix"):
            return None
        email = headers.get("Remote-User", "").lower()
        return email if "@" in email else None

    def channels_for(self, email):
        if self.team and self.team["owner"] == email and not self.accounts[email]["disabled"]:
            return [{"id": self.team["id"],
                     "data": {"channelDetail": {"data": "{\"subject\": \"Team\"}"}}}]
        return []

    def contact_answer(self, body):
        # AddCard with no federation domain: a node-bearing identity or an
        # unknown local guid is refused (405); the signature is verified by
        # the server, and by the crypto unit tests, not by this fake.
        try:
            signed = json.loads(base64.b64decode(body["message"]))
            identity = json.loads(signed["value"])
        except (KeyError, ValueError):
            return 400
        if identity.get("node"):
            return 405
        if signed.get("guid") not in {a["guid"] for a in self.accounts.values()}:
            return 405
        return 200

    def handle(self, method, path, query, body, headers, now):
        """(status, json-serializable answer) for one request."""
        if path == "/admin/status" and method == "GET":
            return 200, not self.configured
        if not self.with_roster and path == "/admin/sso/roster":
            return 404, "404 page not found"
        if path == "/admin/status" and method == "PUT":
            # the admin client sends the token as a query parameter
            self.admin_password = query.get("token")
            self.configured = True
            return 200, None
        if path == "/admin/access":
            if query.get("token") != self.admin_password:
                return 401, None
            return 200, self.admin_session
        if path == "/admin/config" and method == "PUT":
            return 200, None
        if path == "/admin/sso/roster" and method == "PUT":
            if query.get("token") != self.admin_session:
                return 401, None
            return 200, self.push((body or {}).get("owner"), (body or {}).get("emails") or [])
        if path == "/account/sso" and method == "POST":
            email = self.verify(headers, now)
            if email is None:
                return 401, "no verified outpost identity"
            if self.roster is not None and email not in self.roster:
                return 403, "not on roster"
            account = self.accounts.setdefault(
                email, {"guid": self._guid(email), "disabled": False, "tokens": []})
            if account["disabled"]:
                return 403, "account disabled"
            code = "code-%s" % email
            self.codes[code] = email
            return 200, code
        if path == "/account/access" and method == "PUT":
            email = self.codes.pop(query.get("token"), None)
            if email is None:
                return 401, None
            token = "apptok-%s" % email
            self.accounts[email]["tokens"].append(token)
            return 200, {"guid": self.accounts[email]["guid"],
                         "appToken": self.accounts[email]["guid"] + "." + token}
        if path == "/content/channels" and method == "GET":
            email = self.email_for_agent(query.get("agent", ""))
            if email is None:
                return 401, None
            return 200, self.channels_for(email)
        if path == "/contact/cards" and method == "GET":
            email = self.email_for_agent(query.get("agent", ""))
            if email is None:
                return 401, None
            return 200, [{"id": "card-%s-%s" % (email, c["guid"]),
                          "data": {"cardProfile": {"guid": c["guid"]},
                                   "cardDetail": {"status": c["status"]}}}
                         for c in self.cards.get(email, [])]
        if path == "/contact/cards" and method == "POST":
            email = self.email_for_agent(query.get("agent", ""))
            if email is None:
                return 401, None
            return self.contact_answer(body or {}), None
        return 404, None

    def email_for_agent(self, agent):
        guid, _, token = (agent or "").partition(".")
        for email, account in self.accounts.items():
            if account["guid"] == guid and token in account["tokens"]:
                return email
        return None


class InProcessJourney(unittest.TestCase):
    """The real Journey and the real steps against an in-process server."""

    def setUp(self):
        self.secret = "in-process-sso-secret"

    def _serve(self, fake):
        import threading
        from http.server import BaseHTTPRequestHandler, HTTPServer
        from urllib.parse import urlparse, parse_qs
        import time as time_mod

        outer = self

        class H(BaseHTTPRequestHandler):
            def log_message(self, *a):
                pass

            def _run(self, method):
                u = urlparse(self.path)
                q = {k: v[0] for k, v in parse_qs(u.query).items()}
                n = int(self.headers.get("Content-Length") or 0)
                raw = self.rfile.read(n) if n else b""
                try:
                    body = json.loads(raw) if raw else None
                except ValueError:
                    body = raw.decode("utf-8", "replace")
                headers = {k: v for k, v in self.headers.items()}
                status, answer = fake.handle(method, u.path, q, body, headers,
                                             int(time_mod.time()))
                payload = json.dumps(answer).encode() if answer is not None else b""
                self.send_response(status)
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                if payload:
                    self.wfile.write(payload)

            def do_GET(self):
                self._run("GET")

            def do_PUT(self):
                self._run("PUT")

            def do_POST(self):
                self._run("POST")

        server = HTTPServer(("127.0.0.1", 0), H)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(thread.join, timeout=5)
        return f"http://127.0.0.1:{server.server_port}/"

    def test_full_journey_passes_against_the_fake_server(self):
        fake = FakeKlatch(self.secret)
        url = self._serve(fake)
        out = r.run_journey(make_args(), url, self.secret)
        self.assertEqual(r.summarize(out["steps"]), "PASS", out["steps"])
        checks = [c for s in out["steps"].values() for c in s.get("checks", [])]
        self.assertEqual(checks, [
            "sandbox_store_fresh", "roster_endpoint_present", "admin_configured",
            "first_push_counts", "repeat_push_noop",
            "owner_signin", "member_signin", "guids_distinct", "unlisted_refused",
            "unsigned_refused",
            "team_owned_by_roster_owner", "member_not_team_owner", "members_meshed",
            "removal_counts", "removed_contacts_dropped", "removed_signin_refused",
            "disabled_proven_by_reenable", "restored_signin_works",
            "external_node_refused", "unknown_guid_refused"])
        self.assertEqual([p[0] for p in fake.pushes[0:2]],
                         [fake.pushes[0][0]] * 2)
        self.assertEqual(len(out["accounts"]), 2)
        self.assertNotIn(self.secret, json.dumps(out, default=str))

    def test_image_without_the_endpoint_is_blocked_with_instructions(self):
        fake = FakeKlatch(self.secret, with_roster=False)
        url = self._serve(fake)
        out = r.run_journey(make_args(), url, self.secret)
        self.assertEqual(r.summarize(out["steps"]), "BLOCKED")
        reason = out["steps"]["setup"]["reason"]
        self.assertIn("1.2.7", reason)
        self.assertIn("oci build", reason)
        self.assertEqual(out["steps"]["federation"]["reason"], "skipped: setup blocked")
        self.assertEqual(fake.pushes, [])

    def test_claimed_store_is_blocked_before_any_push(self):
        fake = FakeKlatch(self.secret)
        fake.configured = True  # admin/status answers false: a claimed store
        url = self._serve(fake)
        out = r.run_journey(make_args(), url, self.secret)
        self.assertEqual(r.summarize(out["steps"]), "BLOCKED")
        self.assertIn("already claimed", out["steps"]["setup"]["reason"])
        self.assertEqual(fake.pushes, [])


if __name__ == "__main__":
    unittest.main()
