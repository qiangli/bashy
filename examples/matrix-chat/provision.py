#!/usr/bin/env python3
"""Small protocol adapter for the native Bash# Matrix chat command.

Only references to passwords are stored. The tuwunel offline admin console
requires a password in its argv; subprocess output is never echoed for that
operation. Matrix client credentials travel in request bodies or headers.
"""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request


def fail(message):
    raise SystemExit(message)


def run(*argv, capture=False):
    return subprocess.run(argv, check=True, text=True,
                          stdout=subprocess.PIPE if capture else None,
                          stderr=subprocess.PIPE if capture else None)


def vault(ref):
    if not re.fullmatch(r"[A-Za-z0-9_./-]+", ref):
        fail("invalid secret reference")
    result = run("bashy", "secret", "show", ref, capture=True)
    return result.stdout.rstrip("\n")


def config(path):
    cfg = json.loads(Path(path).read_text())
    name = cfg.get("server_name", "")
    if not re.fullmatch(r"[A-Za-z0-9.-]+(?::[0-9]+)?", name):
        fail("server_name must be an explicit DNS name, localhost, or host:port")
    data = Path(cfg.get("data_dir", "")).expanduser()
    if not data.is_absolute() or str(data) == "/":
        fail("data_dir must be an absolute, non-root path")
    cfg["data_dir"] = str(data)
    cfg.setdefault("bind_address", "127.0.0.1")
    cfg.setdefault("port", 6167)
    if cfg["bind_address"] != "127.0.0.1" or not isinstance(cfg["port"], int) or not 1 <= cfg["port"] <= 65535:
        fail("setup requires a loopback address and valid port")
    users = cfg.get("users", [])
    if not users or len({u["localpart"] for u in users}) != len(users):
        fail("users must contain distinct localparts; first user is admin")
    for user in users:
        if not re.fullmatch(r"[a-z0-9._=-]+", user["localpart"]) or not user.get("password_secret"):
            fail("each user needs a valid localpart and password_secret")
    rooms = cfg.get("rooms", [])
    if len({r["key"] for r in rooms}) != len(rooms):
        fail("room keys must be distinct")
    cloudbox = cfg.get("cloudbox")
    if cloudbox is not None:
        cloudbox_config(cfg, cloudbox)
    known = {u["localpart"] for u in users}
    for room in rooms:
        if not re.fullmatch(r"[a-z0-9._=-]+", room["key"]):
            fail("invalid room key")
        if room["creator"] not in known or any(u not in known for u in room.get("invitees", [])):
            fail("room participants must be configured users")
    return cfg


CLOUDBOX_DEFAULTS = {
    "port": 6168,
    "tunnel_token_secret": "matrix-chat/cloudbox/tunnel-token",
    "appservice_token_secret": "matrix-chat/appservice/as-token",
    "homeserver_token_secret": "matrix-chat/appservice/hs-token",
    "account_secret_prefix": "matrix-chat/accounts",
}


def cloudbox_config(cfg, cloudbox):
    if not isinstance(cloudbox, dict) or set(cloudbox) - set(CLOUDBOX_DEFAULTS):
        fail("cloudbox accepts only: " + ", ".join(sorted(CLOUDBOX_DEFAULTS)))
    for key, value in CLOUDBOX_DEFAULTS.items():
        cloudbox.setdefault(key, value)
    port = cloudbox["port"]
    if not isinstance(port, int) or not 1 <= port <= 65535 or port == cfg["port"]:
        fail("cloudbox.port must be a valid port distinct from the homeserver port")
    for key in CLOUDBOX_DEFAULTS:
        if key != "port" and not re.fullmatch(r"[A-Za-z0-9_./-]{1,64}", cloudbox[key]):
            fail("cloudbox.%s must be a vault reference" % key)
    # Each account password lives at <prefix>/cb_<32 hex>; vault names stop at 64.
    if len(cloudbox["account_secret_prefix"].rstrip("/")) > 28:
        fail("cloudbox.account_secret_prefix must be at most 28 characters")


def appservice_toml(cfg):
    """Non-secret appservice registration: the exclusive @cb_ namespace only.

    The tokens reach tuwunel as vault-injected environment variables."""
    regex = "@cb_[0-9a-f]{32}:" + re.escape(cfg["server_name"])
    return ("[global.appservice.cloudbox]\n"
            "sender_localpart = \"_cloudbox\"\n"
            "rate_limited = false\n\n"
            "[[global.appservice.cloudbox.users]]\n"
            "regex = %s\n"
            "exclusive = true\n") % json.dumps(regex)


def shim_unit(cfg):
    cloudbox = cfg["cloudbox"]
    # JSON is YAML; the unit names vault references, never a value.
    return {
        "name": "matrix-chat-cloudbox",
        "kind": "service",
        "description": "Cloudbox account shim and client front for the matrix-chat homeserver",
        "exec": ["python3", str(Path(__file__).resolve().with_name("cloudbox_shim.py")), "serve",
                 "--listen", "127.0.0.1:{port}",
                 "--upstream", "http://127.0.0.1:{settings.homeserver_port}",
                 "--server-name", "{settings.server_name}",
                 "--account-prefix", "{settings.account_prefix}"],
        "settings": {"homeserver_port": str(cfg["port"]), "server_name": cfg["server_name"],
                     "account_prefix": cloudbox["account_secret_prefix"]},
        "ports": [{"name": "http", "port": cloudbox["port"], "bind": "127.0.0.1"}],
        "secrets": [{"env": "MATRIX_CHAT_CLOUDBOX_TOKEN", "ref": cloudbox["tunnel_token_secret"]},
                    {"env": "MATRIX_CHAT_AS_TOKEN", "ref": cloudbox["appservice_token_secret"]}],
        "requires": ["matrix-chat"],
        "health": {"http": "/_cloudbox/v1/health", "interval": "10s", "timeout": "3s",
                   "failures": 3, "start_period": "10s"},
        "restart": {"policy": "on-failure"},
        "stop": {"signal": "TERM", "timeout": "10s"},
        # Native clients carry Matrix tokens, not a Cloudbox login; Cloudbox's
        # edge gates every request before the tunnel (cloudbox docs/matrix-chat.md).
        "expose": {"app": {"name": "matrix-chat", "port": "http", "require_login": False}},
        "enabled": False,
    }


def bound(ref):
    result = subprocess.run(["bashy", "secret", "show", ref], stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL)
    return result.returncode == 0 and bool(result.stdout.strip())


def ensure_generated(ref):
    if bound(ref):
        return
    generated = run("bashy", "secret", "gen", capture=True).stdout
    subprocess.run(["bashy", "secret", "set", ref], input=generated, text=True, check=True,
                   stdout=subprocess.DEVNULL)


def cloudbox(cfg):
    """Attach the Cloudbox account shim to an existing setup (S513.4a)."""
    if "cloudbox" not in cfg:
        fail("config has no cloudbox section")
    identity(cfg)
    data = Path(cfg["data_dir"])
    if not (data / "db").exists():
        fail("run setup first; the homeserver has no database yet")
    cb = cfg["cloudbox"]
    if not bound(cb["tunnel_token_secret"]):
        fail("bind the host tunnel secret first: bashy ask --name MATRIX_TOKEN --stdout | "
             "bashy secret set " + cb["tunnel_token_secret"])
    ensure_generated(cb["appservice_token_secret"])
    ensure_generated(cb["homeserver_token_secret"])
    registration = data / "cloudbox-appservice.toml"
    registration.write_text(appservice_toml(cfg))
    registration.chmod(0o600)
    run("bashy", "service", "set", "matrix-chat",
        "--set", "env.TUWUNEL_CONFIG=" + str(registration),
        "--set", "secrets.0.env=TUWUNEL_APPSERVICE__CLOUDBOX__AS_TOKEN",
        "--set", "secrets.0.ref=" + cb["appservice_token_secret"],
        "--set", "secrets.1.env=TUWUNEL_APPSERVICE__CLOUDBOX__HS_TOKEN",
        "--set", "secrets.1.ref=" + cb["homeserver_token_secret"])
    run("bashy", "service", "verify", "matrix-chat")
    unit = data / "matrix-chat-cloudbox.yaml"
    unit.write_text(json.dumps(shim_unit(cfg), indent=2) + "\n")
    run("bashy", "service", "add", "--force", "-f", str(unit))
    run("bashy", "service", "verify", "matrix-chat-cloudbox")
    run("bashy", "service", "restart", "matrix-chat")
    wait_ready(cfg)
    run("bashy", "service", "restart", "matrix-chat-cloudbox")
    for _ in range(30):
        try:
            with urllib.request.urlopen("http://127.0.0.1:%d/_cloudbox/v1/health" % cb["port"],
                                        timeout=3):
                break
        except (urllib.error.URLError, OSError):
            time.sleep(1)
    else:
        fail("Cloudbox shim did not become ready")
    print(json.dumps({"app": "matrix-chat", "shim_port": cb["port"],
                      "namespace": "@cb_*:" + cfg["server_name"]}, sort_keys=True))


def identity(cfg, create=False):
    data = Path(cfg["data_dir"])
    marker = data / "server-name"
    if marker.exists():
        if marker.read_text().strip() != cfg["server_name"]:
            fail("server_name is immutable; existing data has a different name")
    elif (data / "db").exists():
        fail("existing database has no server-name marker; inspect it before setup")
    elif create:
        data.mkdir(parents=True, exist_ok=True, mode=0o700)
        marker.write_text(cfg["server_name"] + "\n")
        marker.chmod(0o600)


def endpoint(cfg):
    return "http://127.0.0.1:%d" % cfg["port"]


def request(cfg, method, path, token=None, body=None, raw=False):
    headers = {}
    if token:
        headers["Authorization"] = "Bearer " + token
    if body is not None:
        if raw:
            payload = body
        else:
            payload = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
    else:
        payload = None
    req = urllib.request.Request(endpoint(cfg) + path, payload, headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=15) as response:
            content = response.read()
            return content if raw else json.loads(content or b"{}")
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode(errors="replace")[:500]
        fail("Matrix HTTP %s on %s: %s" % (exc.code, path, detail))


def login(cfg, user):
    result = request(cfg, "POST", "/_matrix/client/v3/login", body={
        "type": "m.login.password",
        "identifier": {"type": "m.id.user", "user": user["localpart"]},
        "password": vault(user["password_secret"]),
    })
    return result["access_token"]


def installed_binary():
    output = run("bashy", "tool", "install", "tuwunel", capture=True).stdout
    match = re.search(r"installed tuwunel -> (\S+)", output)
    if not match:
        fail("signed tuwunel install did not report its admitted path")
    return match.group(1)


def setup(cfg):
    identity(cfg, create=True)
    binary = installed_binary()
    if not Path(binary).is_file():
        fail("admitted tuwunel binary is missing")
    check = subprocess.run(["bashy", "service", "show", "matrix-chat"],
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    if check.returncode:
        run("bashy", "service", "add", "--optional", "matrix-chat")
    run("bashy", "service", "set", "matrix-chat",
        "--set", "settings.binary=" + binary,
        "--set", "settings.server_name=" + cfg["server_name"],
        "--set", "settings.data_dir=" + cfg["data_dir"],
        "--set", "ports.0.port=" + str(cfg["port"]))
    run("bashy", "service", "verify", "matrix-chat")
    # The offline admin console takes a RocksDB lock. Stop before provisioning.
    subprocess.run(["bashy", "service", "stop", "matrix-chat"],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    admin_env = os.environ.copy()
    admin_env.update(TUWUNEL_SERVER_NAME=cfg["server_name"],
                     TUWUNEL_DATABASE_PATH=str(Path(cfg["data_dir"]) / "db"),
                     TUWUNEL_ALLOW_REGISTRATION="false",
                     TUWUNEL_ALLOW_FEDERATION="false",
                     TUWUNEL_ADDRESS="127.0.0.1", TUWUNEL_PORT=str(cfg["port"]))
    commands = []
    for i, user in enumerate(cfg["users"]):
        commands.extend(["--execute", "users create-user %s %s" %
                         (user["localpart"], vault(user["password_secret"]))])
        if i == 0:
            commands.extend(["--execute", "users make-user-admin @%s:%s" %
                             (user["localpart"], cfg["server_name"])])
    result = subprocess.run([binary, "--test=smoke", "-O", "admin_execute_errors_ignore=true",
                             *commands], env=admin_env, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE)
    if result.returncode:
        fail("tuwunel offline account provisioning failed (output withheld to protect secrets)")
    run("bashy", "service", "start", "matrix-chat")
    wait_ready(cfg)
    provision(cfg)


def wait_ready(cfg):
    for _ in range(30):
        try:
            request(cfg, "GET", "/_matrix/client/versions")
            return
        except (urllib.error.URLError, OSError):
            time.sleep(1)
    fail("Matrix service did not become ready")


def provision(cfg):
    identity(cfg)
    tokens = {u["localpart"]: login(cfg, u) for u in cfg["users"]}
    rooms = {}
    for room in cfg.get("rooms", []):
        alias = "#%s:%s" % (room["key"], cfg["server_name"])
        path = "/_matrix/client/v3/directory/room/" + urllib.parse.quote(alias, safe="")
        try:
            with urllib.request.urlopen(endpoint(cfg) + path, timeout=10) as response:
                room_id = json.load(response)["room_id"]
        except urllib.error.HTTPError as exc:
            if exc.code != 404:
                fail("room alias lookup failed: HTTP %s" % exc.code)
            response = request(cfg, "POST", "/_matrix/client/v3/createRoom",
                               tokens[room["creator"]], {
                                   "preset": "private_chat", "visibility": "private",
                                   "room_alias_name": room["key"],
                                   "name": room.get("name", room["key"]),
                               })
            room_id = response["room_id"]
        rooms[room["key"]] = room_id
        for localpart in room.get("invitees", []):
            # Joining an existing invitation is idempotent; membership is checked first.
            encoded = urllib.parse.quote(room_id, safe="")
            joined = request(cfg, "GET", "/_matrix/client/v3/joined_rooms", tokens[localpart])["joined_rooms"]
            if room_id not in joined:
                request(cfg, "POST", "/_matrix/client/v3/rooms/%s/invite" % encoded,
                        tokens[room["creator"]], {"user_id": "@%s:%s" % (localpart, cfg["server_name"])})
                request(cfg, "POST", "/_matrix/client/v3/join/%s" % encoded,
                        tokens[localpart], {})
    print(json.dumps({"users": ["@%s:%s" % (u["localpart"], cfg["server_name"])
                                for u in cfg["users"]], "rooms": rooms}, sort_keys=True))


def backup(cfg):
    identity(cfg)
    # RocksDB must be quiescent; no live copy is advertised as a valid backup.
    run("bashy", "service", "stop", "matrix-chat")
    data = Path(cfg["data_dir"])
    target = data.parent / (data.name + "-backup-" + time.strftime("%Y%m%dT%H%M%SZ", time.gmtime()))
    if target.exists():
        fail("backup target already exists")
    shutil.copytree(data, target)
    if (target / "server-name").read_text() != (data / "server-name").read_text():
        fail("backup identity verification failed")
    print(target)


def main():
    action, path = sys.argv[1:]
    cfg = config(path)
    if action == "setup":
        setup(cfg)
    elif action == "provision":
        provision(cfg)
    elif action == "cloudbox":
        cloudbox(cfg)
    elif action == "status":
        run("bashy", "service", "status", "matrix-chat")
    elif action == "start":
        identity(cfg)
        run("bashy", "service", "start", "matrix-chat")
    elif action == "stop":
        run("bashy", "service", "stop", "matrix-chat")
    elif action == "backup":
        backup(cfg)
    elif action == "restore":
        fail("restore is an offline, operator-reviewed copy; see README.md")
    else:
        fail("unknown action: " + action)


if __name__ == "__main__":
    main()
