#!/usr/bin/env python3
"""Setup/provision/status adapter for a running Klatch (Databag fork) server.

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
    cfg.setdefault("users", [])
    cfg.setdefault("groups", [])
    return cfg


class Client:
    def __init__(self, base, resolve=vault):
        self.base = base
        self.resolve = resolve

    def call(self, method, path, query=None, body=None, headers=None):
        url = self.base + path
        if query:
            url += "?" + urllib.parse.urlencode(query)
        data = None
        hdrs = dict(headers or {})
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
                        [], {"Authorization": "Basic " + auth})
        return res["guid"], res["appToken"]

    def identity(self, guid):
        return self.call("GET", "/account/listing/%s/message" % guid)

    def cards(self, agent):
        return self.call("GET", "/contact/cards", {"agent": agent}) or []

    def add_card(self, agent, message):
        res = self.call("POST", "/contact/cards", {"agent": agent}, message)
        return res.get("id") if isinstance(res, dict) else res

    def channels(self, agent):
        return self.call("GET", "/content/channels", {"agent": agent}) or []

    def add_channel(self, agent, subject, card_ids):
        body = {"dataType": "superbasic", "data": json.dumps({"subject": subject}),
                "groups": [], "cards": card_ids}
        res = self.call("POST", "/content/channels", {"agent": agent}, body)
        return res.get("id") if isinstance(res, dict) else res


def account_handle(entry):
    return entry.get("username") or entry.get("handle") or ""


def card_guid(entry):
    data = entry.get("data") or {}
    prof = data.get("cardProfile") or (data.get("card") or {}).get("profile") or {}
    return prof.get("guid")


def channel_subject(entry):
    data = entry.get("data") or {}
    detail = data.get("channelDetail") or {}
    try:
        return json.loads(detail.get("data", "")).get("subject")
    except (ValueError, AttributeError, TypeError):
        return None


def admin_session(client, cfg, bootstrap):
    password = client.resolve(cfg["admin_password_secret"])
    if client.bootstrap_needed():
        if not bootstrap:
            fail("server is not bootstrapped; run setup")
        client.setup_admin(password)
    return client.admin_session(password), password


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

    groups = {}
    for g in cfg["groups"]:
        token = logins[g["owner"]][1]
        found = [c for c in client.channels(token) if channel_subject(c) == g["name"]]
        if found:
            groups[g["key"]] = found[0].get("id")
        else:
            groups[g["key"]] = client.add_channel(
                token, g["name"], [cardids[g["owner"]][m] for m in g.get("members", [])])
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
        for c in client.channels(token):
            if channel_subject(c) == g["name"]:
                groups[g["key"]] = c.get("id")
    out["groups"] = groups
    return out


def main(argv):
    if len(argv) != 2 or argv[0] not in ("setup", "provision", "status"):
        fail("Usage: provision.py setup|provision|status CONFIG.json")
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
