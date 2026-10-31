#!/usr/bin/env python3
"""Setup/provision/status/post/topic adapter for a running Klatch (Databag fork) server.

Passwords are vault references resolved with `bashy secret show`; they travel
only in HTTP headers/query parameters the server contract requires, never in
argv or output.
"""
import base64
import json
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

CONFIG_BODY = {
    "keyType": "RSA2048", "accountStorage": 1073741824, "enableImage": True,
    "enableAudio": False, "enableVideo": False, "enableBinary": True,
    "allowUnsealed": True, "pushSupported": False,
    "enableOpenAccess": False, "openAccessLimit": 0,
}


def fail(message):
    raise SystemExit(message)


def vault(ref):
    if not re.fullmatch(r"[A-Za-z0-9_./-]+", ref):
        fail("invalid secret reference")
    out = subprocess.run(["bashy", "secret", "show", ref], check=True, text=True,
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    return out.stdout.rstrip("\n")


def config(path):
    cfg = json.loads(Path(path).read_text())
    base = cfg.get("base_url", "")
    if not re.fullmatch(r"https?://[^\s/]+(/[^\s?#]*)?", base):
        fail("base_url must be an http(s) URL")
    cfg["base_url"] = base.rstrip("/")
    users = cfg.get("users", [])
    handles = [u.get("handle", "") for u in users]
    if len(set(handles)) != len(handles):
        fail("user handles must be distinct")
    for u in users:
        if not re.fullmatch(r"[A-Za-z0-9._-]+", u.get("handle", "")) or not u.get("password_secret"):
            fail("each user needs a valid handle and password_secret")
    groups = cfg.get("groups", [])
    keys = [g.get("key", "") for g in groups]
    if len(set(keys)) != len(keys) or not all(keys):
        fail("group keys must be distinct")
    for g in groups:
        for h in [g.get("owner")] + list(g.get("members", [])):
            if h not in handles:
                fail("group %s references unknown handle %s" % (g["key"], h))
        if not g.get("name"):
            fail("group %s needs a name" % g["key"])
    bot = cfg.get("bot")
    if bot is not None and bot not in handles:
        fail("bot %s is not a configured user" % bot)
    cfg.setdefault("users", [])
    cfg.setdefault("groups", [])
    return cfg


class Client:
    def __init__(self, base, resolve=vault):
        self.base = base
        self.resolve = resolve
        parts = urllib.parse.urlsplit(base)
        self.origin = "%s://%s" % (parts.scheme, parts.netloc)

    def call(self, method, path, query=None, body=None, headers=None):
        url = self.base + path
        if query:
            url += "?" + urllib.parse.urlencode(query)
        data = None
        hdrs = dict(headers or {})
        hdrs["Origin"] = self.origin
        if body is not None:
            data = json.dumps(body).encode()
            hdrs["Content-Type"] = "application/json"
        req = urllib.request.Request(url, data=data, method=method, headers=hdrs)
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                raw = resp.read().decode()
        except urllib.error.HTTPError as err:
            fail("%s %s -> HTTP %d" % (method, path, err.code))
        except urllib.error.URLError as err:
            fail("%s %s unreachable: %s" % (method, path, err.reason))
        if not raw.strip():
            return None
        return json.loads(raw)

    # --- admin ---
    def bootstrap_needed(self):
        return self.call("GET", "/admin/status") is True

    def admin_session(self, password):
        return self.call("PUT", "/admin/access", {"token": password})

    def setup_admin(self, password):
        self.call("PUT", "/admin/status", {"token": password})

    def set_config(self, session, domain):
        body = dict(CONFIG_BODY, domain=domain)
        self.call("PUT", "/admin/config", {"token": session, "setOpenAccess": "true"}, body)

    def accounts(self, session):
        return self.call("GET", "/admin/accounts", {"token": session}) or []

    def create_account(self, session, handle, password):
        token = self.call("POST", "/admin/accounts", {"token": session})
        cred = base64.b64encode(("%s:%s" % (handle, password)).encode()).decode()
        self.call("POST", "/account/profile", {"token": token}, None,
                  {"Credentials": "Basic " + cred})

    # --- account ---
    def login(self, handle, password):
        auth = base64.b64encode(("%s:%s" % (handle, password)).encode()).decode()
        res = self.call("POST", "/account/apps",
                        {"appName": "bashy-klatch", "appVersion": "1", "platform": "cli"},
                        [], {"Credentials": "Basic " + auth})
        return res["guid"], res["appToken"]

    def identity(self, guid):
        return self.call("GET", "/account/listing/%s/message" % guid)

    def cards(self, agent):
        return self.call("GET", "/contact/cards", {"agent": agent}) or []

    def add_card(self, agent, message):
        res = self.call("POST", "/contact/cards", {"agent": agent}, message)
        return res.get("id") if isinstance(res, dict) else res

    def set_card_status(self, agent, card_id, status, extra=None):
        query = {"agent": agent}
        query.update(extra or {})
        return self.call("PUT", "/contact/cards/%s/status" % card_id, query, status)

    def open_message(self, agent, card_id):
        return self.call("GET", "/contact/cards/%s/openMessage" % card_id, {"agent": agent})

    def deliver_open_message(self, message):
        return self.call("PUT", "/contact/openMessage", None, message)

    def reset_token(self, admin_password, account_id):
        # the reset endpoint takes the admin password itself, not the access session token
        return self.call("PUT", "/admin/accounts/%s/reset" % account_id, {"token": admin_password})

    def set_auth(self, reset_token, handle, password):
        cred = base64.b64encode(("%s:%s" % (handle, password)).encode()).decode()
        self.call("PUT", "/account/auth", None, None,
                  {"Authorization": "Bearer " + reset_token, "Credentials": "Basic " + cred})

    def channels(self, agent):
        return self.call("GET", "/content/channels", {"agent": agent}) or []

    def add_channel(self, agent, subject, card_ids, key=None):
        data = {"subject": subject}
        if key:
            data["key"] = key
        body = {"dataType": "superbasic", "data": json.dumps(data),
                "groups": [], "cards": card_ids}
        res = self.call("POST", "/content/channels", {"agent": agent}, body)
        return res.get("id") if isinstance(res, dict) else res

    def set_subject(self, agent, channel_id, data):
        self.call("PUT", "/content/channels/%s/subject" % channel_id, {"agent": agent},
                  {"dataType": "superbasic", "data": json.dumps(data)})

    def add_topic(self, agent, channel_id, text):
        self.call("POST", "/content/channels/%s/topics" % channel_id,
                  {"agent": agent, "confirm": "true"},
                  {"dataType": "superbasictopic", "data": json.dumps({"text": text})})


def account_handle(entry):
    return entry.get("username") or entry.get("handle") or ""


def card_guid(entry):
    data = entry.get("data") or {}
    prof = data.get("cardProfile") or (data.get("card") or {}).get("profile") or {}
    return prof.get("guid")


def card_status(entry):
    data = entry.get("data") or {}
    return (data.get("cardDetail") or {}).get("status")


def channel_data(entry):
    data = entry.get("data") or {}
    detail = data.get("channelDetail") or {}
    try:
        out = json.loads(detail.get("data", ""))
    except (ValueError, TypeError):
        return {}
    return out if isinstance(out, dict) else {}


def channel_subject(entry):
    return channel_data(entry).get("subject")


def group_channel(channels, g):
    """The group's channel: tagged with its key, else (pre-key) titled by its name."""
    for c in channels:
        if channel_data(c).get("key") == g["key"]:
            return c
    for c in channels:
        if channel_subject(c) == g["name"]:
            return c
    return None


def admin_session(client, cfg, bootstrap):
    password = client.resolve(cfg["admin_password_secret"])
    if client.bootstrap_needed():
        if not bootstrap:
            fail("server is not bootstrapped; run setup")
        client.setup_admin(password)
    return client.admin_session(password), password


LIVE = ("connecting", "connected")


def connect(client, logins, cardids, a, b):
    """Complete the open-message handshake between a's and b's cards (idempotent)."""
    ta, tb = logins[a][1], logins[b][1]
    ca, cb = cardids[a][b], cardids[b][a]

    def state(token, cid):
        for entry in client.cards(token):
            if entry.get("id") == cid:
                return card_status(entry)

    if state(ta, ca) == "connected" and state(tb, cb) == "connected":
        return
    if state(ta, ca) not in LIVE:
        client.set_card_status(ta, ca, "connecting")
    if state(tb, cb) != "connected":
        client.deliver_open_message(client.open_message(ta, ca))
    if state(tb, cb) not in LIVE:
        client.set_card_status(tb, cb, "connecting")
    res = None
    if state(ta, ca) != "connected":
        res = client.deliver_open_message(client.open_message(tb, cb))
    if state(tb, cb) != "connected":
        res = res or {}
        extra = {"token": res.get("token", "")}
        for key in ("viewRevision", "articleRevision", "channelRevision", "profileRevision"):
            extra[key] = res.get(key, 0)
        client.set_card_status(tb, cb, "connected", extra)
    if state(ta, ca) != "connected" or state(tb, cb) != "connected":
        fail("handshake between %s and %s did not reach connected" % (a, b))


def rotate_secret(ref):
    if not re.fullmatch(r"[A-Za-z0-9_./-]+", ref):
        fail("invalid secret reference")
    gen = subprocess.Popen(["bashy", "secret", "gen"], stdout=subprocess.PIPE,
                           stderr=subprocess.DEVNULL)
    put = subprocess.run(["bashy", "secret", "set", ref], stdin=gen.stdout,
                         stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    gen.stdout.close()
    if gen.wait() != 0 or put.returncode != 0:
        fail("could not rotate secret %s" % ref)


def reset_passwords(client, cfg, session, admin_password, handles, rotate=rotate_secret):
    refs = {u["handle"]: u["password_secret"] for u in cfg["users"]}
    ids = {account_handle(a): a.get("accountId") for a in client.accounts(session)}
    for h in handles:
        if h not in refs or ids.get(h) is None:
            fail("unknown handle %s" % h)
    for h in handles:
        rotate(refs[h])
        client.set_auth(client.reset_token(admin_password, ids[h]), h, client.resolve(refs[h]))
    return {"reset": list(handles)}


def provision(client, cfg, session):
    existing = {account_handle(a) for a in client.accounts(session)}
    for u in cfg["users"]:
        if u["handle"] not in existing:
            client.create_account(session, u["handle"], client.resolve(u["password_secret"]))
    logins = {u["handle"]: client.login(u["handle"], client.resolve(u["password_secret"]))
              for u in cfg["users"]}
    guids = {h: g for h, (g, _) in logins.items()}

    # card ids: cardids[owner][member] = owner's card for member
    cardids = {h: {} for h in guids}
    for h, (_, token) in logins.items():
        for entry in client.cards(token):
            g = card_guid(entry)
            for other, og in guids.items():
                if g == og:
                    cardids[h][other] = entry.get("id")

    def link(a, b):
        if b in cardids[a] or a == b:
            return
        cardids[a][b] = client.add_card(logins[a][1], client.identity(guids[b]))

    for g in cfg["groups"]:
        for m in g.get("members", []):
            link(g["owner"], m)
            link(m, g["owner"])

    done = set()
    for g in cfg["groups"]:
        for m in g.get("members", []):
            pair = tuple(sorted((g["owner"], m)))
            if pair[0] != pair[1] and pair not in done:
                done.add(pair)
                connect(client, logins, cardids, *pair)

    groups = {}
    for g in cfg["groups"]:
        token = logins[g["owner"]][1]
        found = group_channel(client.channels(token), g)
        if found:
            groups[g["key"]] = found.get("id")
        else:
            groups[g["key"]] = client.add_channel(
                token, g["name"], [cardids[g["owner"]][m] for m in g.get("members", [])], g["key"])
    return {"handles": guids, "groups": groups}


def status(client, cfg):
    out = {"base_url": cfg["base_url"], "reachable": False}
    try:
        needed = client.bootstrap_needed()
    except SystemExit as err:
        out["error"] = str(err)
        return out
    out["reachable"] = True
    out["bootstrapped"] = not needed
    if needed:
        return out
    session = client.admin_session(client.resolve(cfg["admin_password_secret"]))
    out["accounts"] = len(client.accounts(session))
    groups = {}
    for g in cfg["groups"]:
        try:
            _, token = client.login(g["owner"], client.resolve(
                next(u["password_secret"] for u in cfg["users"] if u["handle"] == g["owner"])))
        except SystemExit:
            continue
        found = group_channel(client.channels(token), g)
        if found:
            groups[g["key"]] = found.get("id")
    out["groups"] = groups
    return out


def bot_channel(client, cfg, key):
    """Log in as the config's bot (default: the group owner) and find the group's channel."""
    g = next((g for g in cfg["groups"] if g["key"] == key), None)
    if g is None:
        fail("unknown group %s" % key)
    bot = cfg.get("bot") or g["owner"]
    ref = next(u["password_secret"] for u in cfg["users"] if u["handle"] == bot)
    _, token = client.login(bot, client.resolve(ref))
    found = group_channel(client.channels(token), g)
    if found is None:
        fail("bot %s does not host group %s; make it the group owner and provision" % (bot, key))
    return g, token, found


def post(client, cfg, key, text):
    _, token, channel = bot_channel(client, cfg, key)
    client.add_topic(token, channel.get("id"), text)
    return {"group": key, "ok": True}


def topic(client, cfg, key, text):
    """Set the channel subject (the conversation header) to 'NAME — TEXT'; empty TEXT restores NAME."""
    g, token, channel = bot_channel(client, cfg, key)
    text = " ".join(text.split())
    data = channel_data(channel)
    data["key"] = key
    data["subject"] = "%s \u2014 %s" % (g["name"], text) if text else g["name"]
    client.set_subject(token, channel.get("id"), data)
    return {"group": key, "ok": True, "subject": data["subject"]}


def message_text(arg, stdin=sys.stdin):
    text = stdin.read() if arg == "-" else arg
    return text.strip()


def main(argv):
    if len(argv) >= 3 and argv[0] == "reset-passwords":
        cfg = config(argv[1])
        client = Client(cfg["base_url"])
        session, admin_pw = admin_session(client, cfg, bootstrap=False)
        print(json.dumps(reset_passwords(client, cfg, session, admin_pw, argv[2:]), indent=2))
        return
    if len(argv) == 4 and argv[0] in ("post", "topic"):
        action, path, key, arg = argv
        cfg = config(path)
        text = message_text(arg)
        if action == "post" and not text:
            fail("post needs non-empty TEXT")
        fn = post if action == "post" else topic
        print(json.dumps(fn(Client(cfg["base_url"]), cfg, key, text), sort_keys=True))
        return
    if len(argv) != 2 or argv[0] not in ("setup", "provision", "status"):
        fail("Usage: provision.py setup|provision|status CONFIG.json\n"
             "       provision.py post|topic CONFIG.json GROUP TEXT|-\n"
             "       provision.py reset-passwords CONFIG.json HANDLE...")
    action, path = argv
    cfg = config(path)
    client = Client(cfg["base_url"])
    if action == "status":
        result = status(client, cfg)
    else:
        session, _ = admin_session(client, cfg, bootstrap=(action == "setup"))
        if action == "setup":
            if not cfg.get("domain"):
                fail("setup requires domain")
            client.set_config(session, cfg["domain"])
        result = provision(client, cfg, session)
    print(json.dumps(result, indent=2, sort_keys=True))


if __name__ == "__main__":
    main(sys.argv[1:])
