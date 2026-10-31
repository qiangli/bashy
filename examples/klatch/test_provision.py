import base64
import importlib.util
import json
import re
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

spec = importlib.util.spec_from_file_location("klatch_provision", Path(__file__).with_name("provision.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

ADMIN = "adminpw"
SECRETS = {"klatch/admin": ADMIN, "klatch/bot": "botpw", "klatch/alice": "apw", "klatch/bob": "bpw", "klatch/carol": "cpw"}


class State:
    def __init__(self):
        self.admin = None
        self.config = None
        self.accounts = {}      # handle -> (password, guid, token)
        self.creates = {}       # create token -> True
        self.cards = {}         # app token -> [{id, data}]
        self.channels = {}      # app token -> [{id, data}]
        self.profile_headers = []
        self.origins = set()
        self.resets = {}        # reset token -> handle
        self.next = 0
        self.connected_calls = []
        self.topics = []        # (agent, channel id, query, body)
        self.subjects = []      # (agent, channel id, body)

    def nid(self):
        self.next += 1
        return "id%d" % self.next


def make_handler(st):
    class H(BaseHTTPRequestHandler):
        def log_message(self, *a):
            pass

        def reply(self, obj, code=200):
            raw = json.dumps(obj).encode()
            self.send_response(code)
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        def handle_any(self, method):
            u = urlparse(self.path)
            q = {k: v[0] for k, v in parse_qs(u.query).items()}
            n = int(self.headers.get("Content-Length") or 0)
            body = json.loads(self.rfile.read(n)) if n else None
            p = u.path
            st.origins.add(self.headers.get("Origin"))
            by_agent = {t: h for h, (_, _, t) in st.accounts.items()}
            guid_agent = {g: t for _, (_, g, t) in st.accounts.items()}

            def find_card(agent, cid):
                return next(c for c in st.cards.get(agent, []) if c["id"] == cid)
            by_token = {t: h for h, (_, _, t) in st.accounts.items()}
            if p == "/admin/status" and method == "GET":
                return self.reply(st.admin is None)
            if p == "/admin/status" and method == "PUT":
                st.admin = q["token"]
                return self.reply(None)
            if p == "/admin/access":
                return self.reply("SESSION") if q.get("token") == st.admin else self.reply(None, 401)
            if p == "/admin/config" and method == "PUT":
                st.config = (q, body)
                return self.reply(None)
            if p == "/admin/accounts" and method == "GET":
                return self.reply([{"username": h, "accountId": i + 1} for i, h in enumerate(st.accounts)])
            if p.startswith("/admin/accounts/") and p.endswith("/reset"):
                if q.get("token") != st.admin:
                    return self.reply(None, 401)
                handle = list(st.accounts)[int(p.split("/")[3]) - 1]
                tok = "reset-" + st.nid()
                st.resets[tok] = handle
                return self.reply(tok)
            if p == "/account/auth" and method == "PUT":
                handle = st.resets.pop(self.headers["Authorization"].split()[1], None)
                if not handle:
                    return self.reply(None, 401)
                user, pw = base64.b64decode(self.headers["Credentials"].split()[1]).decode().split(":", 1)
                assert user == handle
                _, guid, tok = st.accounts[handle]
                st.accounts[handle] = (pw, guid, tok)
                return self.reply(None)
            if p == "/admin/accounts" and method == "POST":
                tok = "create-" + st.nid()
                st.creates[tok] = True
                return self.reply(tok)
            if p == "/account/profile":
                cred = self.headers.get("Credentials")
                st.profile_headers.append((cred, self.headers.get("Authorization")))
                user, pw = base64.b64decode(cred.split()[1]).decode().split(":", 1)
                st.accounts[user] = (pw, "guid-" + user, "tok-" + user)
                return self.reply(None)
            if p == "/account/apps":
                user, pw = base64.b64decode(self.headers["Credentials"].split()[1]).decode().split(":", 1)
                acct = st.accounts.get(user)
                if not acct or acct[0] != pw:
                    return self.reply(None, 401)
                return self.reply({"guid": acct[1], "appToken": acct[2]})
            if p.startswith("/account/listing/"):
                return self.reply({"guid": p.split("/")[3], "sig": "x"})
            if p == "/contact/cards" and method == "GET":
                return self.reply(st.cards.get(q["agent"], []))
            if p == "/contact/cards" and method == "POST":
                cid = st.nid()
                st.cards.setdefault(q["agent"], []).append(
                    {"id": cid, "in": "in-" + cid, "data": {"cardProfile": {"guid": body["guid"]},
                                                           "cardDetail": {"status": "confirmed"}}})
                return self.reply({"id": cid})
            m = re.fullmatch(r"/contact/cards/([^/]+)/status", p)
            if m and method == "PUT":
                c = find_card(q["agent"], m.group(1))
                if body == "connected" and not q.get("token"):
                    return self.reply(None, 400)
                if body == "connected":
                    c["out"] = q["token"]
                    st.connected_calls.append(dict(q))
                c["data"]["cardDetail"]["status"] = body
                return self.reply(c)
            m = re.fullmatch(r"/contact/cards/([^/]+)/openMessage", p)
            if m:
                c = find_card(q["agent"], m.group(1))
                if c["data"]["cardDetail"]["status"] not in ("connecting", "connected"):
                    return self.reply(None, 405)
                return self.reply({"from": by_agent[q["agent"]], "to": c["data"]["cardProfile"]["guid"],
                                   "token": c["in"]})
            if p == "/contact/openMessage" and method == "PUT":
                agent = guid_agent[body["to"]]
                src = "guid-" + body["from"]
                c = next(c for c in st.cards[agent] if c["data"]["cardProfile"]["guid"] == src)
                d = c["data"]["cardDetail"]
                if d["status"] == "confirmed":
                    d["status"] = "requested"
                elif d["status"] == "connecting":
                    d["status"] = "connected"
                c["out"] = body["token"]
                return self.reply({"token": c["in"], "viewRevision": 1, "articleRevision": 2,
                                   "channelRevision": 3, "profileRevision": 4})
            if p == "/content/channels" and method == "GET":
                return self.reply(st.channels.get(q["agent"], []))
            if p == "/content/channels" and method == "POST":
                cid = st.nid()
                st.channels.setdefault(q["agent"], []).append(
                    {"id": cid, "data": {"channelDetail": {"dataType": body["dataType"], "data": body["data"]}},
                     "cards": body["cards"]})
                return self.reply({"id": cid})
            m = re.fullmatch(r"/content/channels/([^/]+)/(subject|topics)", p)
            if m:
                c = next((c for c in st.channels.get(q.get("agent"), []) if c["id"] == m.group(1)), None)
                if c is None:
                    return self.reply(None, 404)
                if m.group(2) == "subject" and method == "PUT":
                    st.subjects.append((q["agent"], c["id"], body))
                    c["data"]["channelDetail"].update(body)
                    return self.reply(None)
                if m.group(2) == "topics" and method == "POST":
                    st.topics.append((q["agent"], c["id"], q, body))
                    return self.reply({"id": st.nid()})
            self.reply(None, 404)

        def do_GET(self):
            self.handle_any("GET")

        def do_PUT(self):
            self.handle_any("PUT")

        def do_POST(self):
            self.handle_any("POST")

    return H


class ProvisionTest(unittest.TestCase):
    def setUp(self):
        self.st = State()
        self.srv = HTTPServer(("127.0.0.1", 0), make_handler(self.st))
        threading.Thread(target=self.srv.serve_forever, daemon=True).start()
        self.cfg = {
            "base_url": "http://127.0.0.1:%d" % self.srv.server_port, "domain": "chat.test",
            "admin_password_secret": "klatch/admin",
            "users": [{"handle": h, "password_secret": "klatch/" + h} for h in ("alice", "bob", "carol")],
            "groups": [{"key": "session", "name": "Session", "owner": "alice", "members": ["bob", "carol"]}],
        }
        self.client = module.Client(self.cfg["base_url"], resolve=SECRETS.__getitem__)

    def tearDown(self):
        self.srv.shutdown()
        self.srv.server_close()

    def run_setup(self):
        session, _ = module.admin_session(self.client, self.cfg, bootstrap=True)
        self.client.set_config(session, self.cfg["domain"])
        return module.provision(self.client, self.cfg, session)

    def test_bootstrap_and_provision(self):
        out = self.run_setup()
        self.assertEqual(self.st.admin, ADMIN)
        q, body = self.st.config
        self.assertEqual(q["setOpenAccess"], "true")
        self.assertEqual(body["domain"], "chat.test")
        self.assertFalse(body["enableOpenAccess"])
        self.assertEqual(out["handles"], {h: "guid-" + h for h in ("alice", "bob", "carol")})
        self.assertEqual(list(out["groups"]), ["session"])
        # alice holds cards for bob and carol; each member holds one for alice
        self.assertEqual(len(self.st.cards["tok-alice"]), 2)
        self.assertEqual(len(self.st.cards["tok-bob"]), 1)
        self.assertEqual(len(self.st.channels["tok-alice"][0]["cards"]), 2)

    def statuses(self):
        return {(a, i): c["data"]["cardDetail"]["status"]
                for a, cs in self.st.cards.items() for i, c in enumerate(cs)}

    def test_handshake_connects_all_pairs(self):
        self.run_setup()
        self.assertEqual(set(self.statuses().values()), {"connected"})
        self.assertEqual(len(self.statuses()), 4)
        self.assertEqual(len(self.st.connected_calls), 2)
        call = self.st.connected_calls[0]
        self.assertTrue(call["token"].startswith("in-"))
        self.assertEqual((call["viewRevision"], call["profileRevision"]), ("1", "4"))
        self.assertEqual(self.st.origins, {self.cfg["base_url"]})

    def test_handshake_idempotent(self):
        self.run_setup()
        before = json.dumps(self.st.cards, sort_keys=True)
        calls = len(self.st.connected_calls)
        self.run_setup()
        self.assertEqual(json.dumps(self.st.cards, sort_keys=True), before)
        self.assertEqual(len(self.st.connected_calls), calls)

    def test_reset_passwords(self):
        session, _ = module.admin_session(self.client, self.cfg, bootstrap=True)
        self.client.set_config(session, self.cfg["domain"])
        module.provision(self.client, self.cfg, session)
        rotated = []

        def rotate(ref):
            rotated.append(ref)
            SECRETS[ref] = "new-" + ref

        try:
            out = module.reset_passwords(self.client, self.cfg, session, ADMIN, ["bob", "carol"], rotate)
            self.assertEqual(out, {"reset": ["bob", "carol"]})
            self.assertEqual(rotated, ["klatch/bob", "klatch/carol"])
            self.assertEqual(self.st.accounts["bob"][0], "new-klatch/bob")
            self.assertEqual(self.st.accounts["alice"][0], "apw")
            self.assertTrue(self.client.login("bob", "new-klatch/bob"))
            with self.assertRaises(SystemExit):
                module.reset_passwords(self.client, self.cfg, session, ADMIN, ["zed"], rotate)
        finally:
            SECRETS.update({"klatch/bob": "bpw", "klatch/carol": "cpw"})

    def test_credentials_header_not_authorization(self):
        self.run_setup()
        self.assertEqual(len(self.st.profile_headers), 3)
        for cred, auth in self.st.profile_headers:
            self.assertTrue(cred.startswith("Basic "))
            self.assertIsNone(auth)

    def test_rerun_no_duplicates(self):
        first = self.run_setup()
        second = self.run_setup()
        self.assertEqual(first, second)
        self.assertEqual(len(self.st.accounts), 3)
        self.assertEqual(len(self.st.cards["tok-alice"]), 2)
        self.assertEqual(len(self.st.cards["tok-bob"]), 1)
        self.assertEqual(len(self.st.channels["tok-alice"]), 1)
        self.assertEqual(len(self.st.creates), 3)

    def test_json_string_decoding(self):
        self.assertIs(self.client.bootstrap_needed(), True)
        module.admin_session(self.client, self.cfg, bootstrap=True)
        self.assertIs(self.client.bootstrap_needed(), False)
        self.assertEqual(self.client.admin_session(ADMIN), "SESSION")

    def test_provision_requires_bootstrap(self):
        with self.assertRaises(SystemExit):
            module.admin_session(self.client, self.cfg, bootstrap=False)

    def test_invite_links_go_to_vault_only(self):
        self.run_setup()
        session, _ = module.admin_session(self.client, self.cfg, bootstrap=False)
        stored = {}
        out = module.invite(self.client, self.cfg, session, 3, "klatch/signup-links",
                            store=lambda ref, value: stored.update({ref: value}))
        self.assertEqual(out, {"links": 3, "ref": "klatch/signup-links"})
        links = stored["klatch/signup-links"].split()
        self.assertEqual(len(links), 3)
        self.assertEqual(len(set(links)), 3)
        for link in links:
            self.assertTrue(link.startswith(self.cfg["base_url"] + "/?add=create-"), link)
            self.assertNotIn(link.split("=", 1)[1], json.dumps(out))
        self.assertEqual(len(self.st.creates), 3 + 3)  # 3 provisioned accounts + 3 unused invites

    def test_invite_count_bounds(self):
        self.run_setup()
        session, _ = module.admin_session(self.client, self.cfg, bootstrap=False)
        for bad in (0, 101):
            with self.assertRaises(SystemExit):
                module.invite(self.client, self.cfg, session, bad, "klatch/x", store=lambda *a: None)

    def test_status(self):
        self.assertFalse(module.status(self.client, self.cfg)["bootstrapped"])
        self.run_setup()
        s = module.status(self.client, self.cfg)
        self.assertTrue(s["reachable"])
        self.assertEqual(s["accounts"], 3)
        self.assertEqual(list(s["groups"]), ["session"])

    def test_status_unreachable(self):
        dead = module.Client("http://127.0.0.1:1", resolve=SECRETS.__getitem__)
        self.assertFalse(module.status(dead, dict(self.cfg, base_url="http://127.0.0.1:1"))["reachable"])

    def test_config_rejects_unknown_member(self):
        import tempfile
        bad = dict(self.cfg, groups=[{"key": "g", "name": "G", "owner": "alice", "members": ["zed"]}])
        with tempfile.TemporaryDirectory() as t:
            p = Path(t) / "c.json"
            p.write_text(json.dumps(bad))
            with self.assertRaises(SystemExit):
                module.config(p)



class ChatTest(unittest.TestCase):
    """post/topic act as the config's bot, which hosts the group."""
    tearDown = ProvisionTest.tearDown
    run_setup = ProvisionTest.run_setup

    def setUp(self):
        ProvisionTest.setUp(self)
        self.cfg["users"].append({"handle": "bot", "password_secret": "klatch/bot"})
        self.cfg["bot"] = "bot"
        self.cfg["groups"][0]["owner"] = "bot"
        self.cfg["groups"][0]["members"] = ["alice", "bob", "carol"]
        self.out = self.run_setup()
        self.chan = self.out["groups"]["session"]

    def test_post_as_bot(self):
        out = module.post(self.client, self.cfg, "session", "step-02 started")
        self.assertEqual(out, {"group": "session", "ok": True})
        agent, cid, q, body = self.st.topics[-1]
        self.assertEqual((agent, cid, q["confirm"]), ("tok-bot", self.chan, "true"))
        self.assertEqual(body["dataType"], "superbasictopic")
        self.assertEqual(json.loads(body["data"]), {"text": "step-02 started"})

    def test_topic_sets_header_and_keeps_group(self):
        out = module.topic(self.client, self.cfg, "session", "step-02 Pair up \u00b7 12m left\n")
        self.assertEqual(out["subject"], "Session \u2014 step-02 Pair up \u00b7 12m left")
        agent, cid, body = self.st.subjects[-1]
        self.assertEqual((agent, cid), ("tok-bot", self.chan))
        self.assertEqual(json.loads(body["data"]), {"subject": out["subject"], "key": "session"})
        # renamed header must not make a rerun create a duplicate group
        self.assertEqual(self.run_setup(), self.out)
        self.assertEqual(len(self.st.channels["tok-bot"]), 1)
        module.topic(self.client, self.cfg, "session", "")
        self.assertEqual(json.loads(self.st.subjects[-1][2]["data"])["subject"], "Session")

    def test_stdin_text(self):
        import io
        self.assertEqual(module.message_text("-", io.StringIO("from stdin\n")), "from stdin")
        self.assertEqual(module.message_text("literal"), "literal")

    def test_bot_must_host_group(self):
        cfg = dict(self.cfg, bot="alice")
        with self.assertRaises(SystemExit):
            module.post(self.client, cfg, "session", "hi")
        with self.assertRaises(SystemExit):
            module.post(self.client, self.cfg, "nope", "hi")
        self.assertEqual(self.st.topics, [])


if __name__ == "__main__":
    unittest.main()


class InviteFlagsTest(unittest.TestCase):
    def test_required_count_and_optional_ref(self):
        from unittest.mock import patch
        import contextlib
        import io
        with patch.object(module, 'config', return_value={'base_url': 'http://localhost'}) as cfg, \
             patch.object(module, 'Client'), \
             patch.object(module, 'admin_session', return_value=('session', 'password')), \
             patch.object(module, 'invite', return_value={'links': 7, 'ref': 'owner/links'}) as mint, \
             contextlib.redirect_stdout(io.StringIO()):
            module.main(['invite', 'owner.json', '--count', '7', '--ref', 'owner/links'])
            self.assertEqual(mint.call_args.args[3:], (7, 'owner/links'))
            module.main(['invite', 'owner.json', '--count', '2'])
            self.assertEqual(mint.call_args.args[3:], (2, 'klatch/signup-links'))
            cfg.reset_mock()
            for args in (['invite', 'owner.json'], ['invite', 'owner.json', '3'],
                         ['invite', 'owner.json', '--count', '0']):
                with self.subTest(args=args), self.assertRaises(SystemExit), contextlib.redirect_stderr(io.StringIO()):
                    module.main(args)
            cfg.assert_not_called()

    def test_registered_invite_flags_reach_validation(self):
        import os
        import subprocess
        import tempfile
        here = Path(__file__).resolve().parent
        with tempfile.TemporaryDirectory(prefix='klatch-registration-') as tmp:
            root = Path(tmp)
            env = dict(os.environ, BASHY_HOME=str(root/'home'),
                       BASHY_COORD_DIR=str(root/'coord'), BASHY_ROOM_DIR=str(root/'room'),
                       XDG_DATA_HOME=str(root/'data'), BASHY_HINTS='off')
            subprocess.run(['git', 'init', '-q', tmp], check=True, env=env)
            registered = subprocess.run(['bash', str(here/'register.sh')], cwd=tmp,
                                        env=env, text=True, capture_output=True)
            self.assertEqual(registered.returncode, 0, registered.stderr)
            for command in (['bashy', 'klatch'], ['bashy', '--bashsharp', str(here/'klatch.bsh')]):
                result = subprocess.run(command + ['invite', 'missing.json', '--count', '0', '--ref', 'owner/links'],
                                        cwd=tmp, env=env, text=True, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('invite count must be 1..100', result.stderr)
                self.assertNotIn('unknown flag', result.stderr)
