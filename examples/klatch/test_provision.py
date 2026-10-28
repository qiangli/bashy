import base64
import importlib.util
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

spec = importlib.util.spec_from_file_location("klatch_provision", Path(__file__).with_name("provision.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

ADMIN = "adminpw"
SECRETS = {"klatch/admin": ADMIN, "klatch/alice": "apw", "klatch/bob": "bpw", "klatch/carol": "cpw"}


class State:
    def __init__(self):
        self.admin = None
        self.config = None
        self.accounts = {}      # handle -> (password, guid, token)
        self.creates = {}       # create token -> True
        self.cards = {}         # app token -> [{id, data}]
        self.channels = {}      # app token -> [{id, data}]
        self.profile_headers = []
        self.next = 0

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
                return self.reply([{"username": h} for h in st.accounts])
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
                user, pw = base64.b64decode(self.headers["Authorization"].split()[1]).decode().split(":", 1)
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
                    {"id": cid, "data": {"cardProfile": {"guid": body["guid"]}}})
                return self.reply({"id": cid})
            if p == "/content/channels" and method == "GET":
                return self.reply(st.channels.get(q["agent"], []))
            if p == "/content/channels" and method == "POST":
                cid = st.nid()
                st.channels.setdefault(q["agent"], []).append(
                    {"id": cid, "data": {"channelDetail": {"dataType": body["dataType"], "data": body["data"]}},
                     "cards": body["cards"]})
                return self.reply({"id": cid})
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
            "groups": [{"key": "class", "name": "Class", "owner": "alice", "members": ["bob", "carol"]}],
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
        self.assertEqual(list(out["groups"]), ["class"])
        # alice holds cards for bob and carol; each member holds one for alice
        self.assertEqual(len(self.st.cards["tok-alice"]), 2)
        self.assertEqual(len(self.st.cards["tok-bob"]), 1)
        self.assertEqual(len(self.st.channels["tok-alice"][0]["cards"]), 2)

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

    def test_status(self):
        self.assertFalse(module.status(self.client, self.cfg)["bootstrapped"])
        self.run_setup()
        s = module.status(self.client, self.cfg)
        self.assertTrue(s["reachable"])
        self.assertEqual(s["accounts"], 3)
        self.assertEqual(list(s["groups"]), ["class"])

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


if __name__ == "__main__":
    unittest.main()
