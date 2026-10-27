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
    known = {u["localpart"] for u in users}
    for room in rooms:
        if not re.fullmatch(r"[a-z0-9._=-]+", room["key"]):
            fail("invalid room key")
        if room["creator"] not in known or any(u not in known for u in room.get("invitees", [])):
            fail("room participants must be configured users")
    return cfg


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
    for _ in range(50):
        stat = subprocess.run(["bashy", "service", "status", "matrix-chat"],
                              capture_output=True, text=True)
        if "inactive" in stat.stdout or "failed" in stat.stdout:
            break
        time.sleep(0.1)
    time.sleep(0.2)
    admin_env = os.environ.copy()
    admin_env.update(TUWUNEL_SERVER_NAME=cfg["server_name"],
                     TUWUNEL_DATABASE_PATH=str(Path(cfg["data_dir"]) / "db"),
                     TUWUNEL_ALLOW_REGISTRATION="false",
                     TUWUNEL_ALLOW_FEDERATION="false",
                     TUWUNEL_ADDRESS="127.0.0.1", TUWUNEL_PORT=str(cfg["port"]))
    list_res = subprocess.run([binary, "--test=smoke", "--execute", "users list-users"],
                              env=admin_env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    existing = set()
    if list_res.returncode:
        fail("tuwunel user enumeration failed; refusing account changes")
    for line in list_res.stdout.splitlines():
        clean = re.sub(r"\x1b\[[0-9;]*[mGKF]", "", line).strip()
        if clean.startswith("@") and ":" in clean:
            existing.add(clean[1:].split(":")[0])
    commands = []
    for i, user in enumerate(cfg["users"]):
        u = user["localpart"]
        pw = vault(user["password_secret"])
        if u not in existing:
            commands.extend(["--execute", f"users create-user {u} {pw}"])
            if i == 0:
                commands.extend(["--execute", "users make-user-admin @%s:%s" %
                                 (u, cfg["server_name"])])
    result = subprocess.run([binary, "--test=smoke",
                             *commands], env=admin_env, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE)
    if result.returncode:
        fail("tuwunel offline account provisioning failed (output withheld to protect secrets)")
    run("bashy", "service", "start", "matrix-chat")
    for _ in range(30):
        try:
            request(cfg, "GET", "/_matrix/client/versions")
            break
        except (urllib.error.URLError, OSError):
            time.sleep(1)
    else:
        fail("Matrix service did not become ready")
    provision(cfg)


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
