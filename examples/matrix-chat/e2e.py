#!/usr/bin/env python3
"""Required live Matrix protocol E2E; fails on missing signed tool/service."""
import base64
import contextlib
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.parse
import urllib.request

here = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("matrix_provision", here / "provision.py")
matrix = importlib.util.module_from_spec(spec)
spec.loader.exec_module(matrix)


def check(value, message):
    if not value:
        raise AssertionError(message)


def transaction():
    return "s513-" + str(time.time_ns())


def find_genuine_bashy():
    if os.environ.get("BASHY_REAL_BIN"):
        p = os.environ["BASHY_REAL_BIN"]
        if os.path.isfile(p) and os.access(p, os.X_OK):
            return str(Path(p).resolve())
    for candidate in [
        shutil.which("bashy"),
        os.path.expanduser("~/.local/bin/bashy"),
        "/opt/homebrew/bin/bashy",
        "/usr/local/bin/bashy",
    ]:
        if candidate and os.path.isfile(candidate) and os.access(candidate, os.X_OK):
            return str(Path(candidate).resolve())
    raise AssertionError("Fail closed: genuine bashy binary absent")


def verify_signed_tuwunel_and_service(real_bashy):
    # Fails closed if signed tuwunel tool or matrix-chat service is absent
    try:
        tool_proc = subprocess.run([real_bashy, "tool", "install", "tuwunel"],
                                  capture_output=True, text=True)
    except (OSError, subprocess.SubprocessError) as err:
        raise AssertionError(f"Fail closed: genuine bashy binary execution failed: {err}") from err

    match = re.search(r"installed tuwunel -> (\S+)", tool_proc.stdout)
    if tool_proc.returncode != 0 or not match or not Path(match.group(1)).is_file():
        raise AssertionError(
            f"Fail closed: signed tuwunel tool absent or admission failed: {tool_proc.stderr or tool_proc.stdout}"
        )

    try:
        svc_proc = subprocess.run([real_bashy, "service", "add", "--optional", "matrix-chat", "--help"],
                                  capture_output=True, text=True)
    except (OSError, subprocess.SubprocessError) as err:
        raise AssertionError(f"Fail closed: genuine bashy service check failed: {err}") from err

    if svc_proc.returncode != 0:
        raise AssertionError(
            f"Fail closed: optional matrix-chat service unit absent: {svc_proc.stderr or svc_proc.stdout}"
        )


@contextlib.contextmanager
def synthetic_secret_adapter(root, real_bashy):
    """Bounded test-only synthetic secret adapter shim.

    Intercepts only `bashy secret` subcommands (gen, set, show, rm),
    storing synthetic per-run credentials in an isolated per-test directory
    with strict permissions (0o700/0o600). Dispatches all other verbs
    to genuine bashy binary. Never exposes secrets in logs or affects
    production vault config.
    """
    secrets_dir = root / "synthetic-secrets"
    secrets_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
    shim_dir = root / "shim-bin"
    shim_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
    shim_path = shim_dir / "bashy"

    shim_code = f"""#!/usr/bin/env python3
import sys, os, secrets, base64, pathlib

real_bashy = {repr(real_bashy)}
secrets_dir = {repr(str(secrets_dir))}
store = pathlib.Path(secrets_dir)

args = sys.argv[1:]
verb_idx = -1
for i, a in enumerate(args):
    if not a.startswith("-"):
        verb_idx = i
        break

if verb_idx != -1 and args[verb_idx] == "secret":
    subargs = args[verb_idx + 1:]
    if not subargs:
        sys.stderr.write("secret requires subcommand\\n")
        sys.exit(2)
    sub = subargs[0]
    if sub == "gen":
        sys.stdout.write(secrets.token_urlsafe(32) + "\\n")
        sys.stdout.flush()
        sys.exit(0)
    elif sub == "set":
        if len(subargs) < 2:
            sys.stderr.write("usage: bashy secret set REF\\n")
            sys.exit(2)
        ref = subargs[1]
        data = sys.stdin.read().rstrip("\\r\\n")
        enc = base64.urlsafe_b64encode(ref.encode("utf-8")).decode("ascii")
        p = store / enc
        p.write_text(data, encoding="utf-8")
        p.chmod(0o600)
        sys.stdout.write(f"set {{ref}}\\n")
        sys.stdout.flush()
        sys.exit(0)
    elif sub == "show":
        if len(subargs) < 2:
            sys.stderr.write("usage: bashy secret show REF\\n")
            sys.exit(2)
        ref = subargs[1]
        enc = base64.urlsafe_b64encode(ref.encode("utf-8")).decode("ascii")
        p = store / enc
        if not p.is_file():
            sys.stderr.write(f"secret: reference not found: {{ref}}\\n")
            sys.exit(1)
        data = p.read_text(encoding="utf-8")
        sys.stdout.write(data + "\\n")
        sys.stdout.flush()
        sys.exit(0)
    elif sub == "rm":
        if len(subargs) < 2:
            sys.stderr.write("usage: bashy secret rm REF\\n")
            sys.exit(2)
        ref = subargs[1]
        enc = base64.urlsafe_b64encode(ref.encode("utf-8")).decode("ascii")
        p = store / enc
        if p.is_file():
            p.unlink()
        sys.stdout.write(f"rm {{ref}}\\n")
        sys.stdout.flush()
        sys.exit(0)
    else:
        sys.stderr.write(f"unsupported synthetic secret subcommand: {{sub}}\\n")
        sys.exit(1)

os.execv(real_bashy, [real_bashy] + args)
"""
    shim_path.write_text(shim_code)
    shim_path.chmod(0o755)

    orig_path = os.environ.get("PATH", "")
    orig_real_bin = os.environ.get("BASHY_REAL_BIN")
    orig_secrets_dir = os.environ.get("BASHY_SYNTHETIC_SECRETS_DIR")

    os.environ["PATH"] = str(shim_dir) + ":" + orig_path
    os.environ["BASHY_REAL_BIN"] = real_bashy
    os.environ["BASHY_SYNTHETIC_SECRETS_DIR"] = str(secrets_dir)
    try:
        yield
    finally:
        os.environ["PATH"] = orig_path
        if orig_real_bin is not None:
            os.environ["BASHY_REAL_BIN"] = orig_real_bin
        else:
            os.environ.pop("BASHY_REAL_BIN", None)
        if orig_secrets_dir is not None:
            os.environ["BASHY_SYNTHETIC_SECRETS_DIR"] = orig_secrets_dir
        else:
            os.environ.pop("BASHY_SYNTHETIC_SECRETS_DIR", None)


def main():
    real_bashy = find_genuine_bashy()
    verify_signed_tuwunel_and_service(real_bashy)

    with tempfile.TemporaryDirectory(prefix="matrix-chat-e2e-") as temp:
        root = Path(temp)
        os.environ["BASHY_HOME"] = str(root / "bashy-home")
        os.environ["BASHY_SERVICES_DIR"] = str(root / "services")
        os.environ["BASHY_FLEET_DIR"] = str(root / "fleet")

        # Socket path must stay short (< 104 bytes) on macOS
        sock_dir = tempfile.mkdtemp(dir="/tmp", prefix="sv-")
        sock_path = os.path.join(sock_dir, "s.sock")
        os.environ["BASHY_SUPERVISORD_SOCKET"] = sock_path

        sup = None
        refs = []
        path = None
        command = [real_bashy, "--bashsharp", str(here / "matrix-chat.bsh")]
        try:
            with synthetic_secret_adapter(root, real_bashy):
                # Disable pre-seeded default services in isolated fleet
                for u in [
                    "actrunner", "apps", "events", "files", "headlamp", "index",
                    "kopia", "lan-inference", "llm", "loom", "meet", "ollama",
                    "ollama-pool", "otel", "otel-stack", "podman", "sandbox",
                    "schedule", "sdlc", "seaweedfs", "shard", "warm", "ycode", "zot", "sshd",
                ]:
                    subprocess.run([real_bashy, "service", "disable", u],
                                   capture_output=True)

                sup = subprocess.Popen([real_bashy, "supervisord"],
                                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                for _ in range(50):
                    if os.path.exists(sock_path):
                        break
                    time.sleep(0.1)
                else:
                    raise AssertionError("Supervisord control socket did not appear")

                with socket.socket() as sock:
                    sock.bind(("127.0.0.1", 0))
                    port = sock.getsockname()[1]
                suffix = str(time.time_ns())
                refs = ["matrix/e2e/%s/%s" % (suffix, u) for u in ("alice", "bob")]
                for ref in refs:
                    generated = subprocess.run(["bashy", "secret", "gen"], check=True,
                                               stdout=subprocess.PIPE).stdout
                    subprocess.run(["bashy", "secret", "set", ref], input=generated,
                                   check=True, stdout=subprocess.DEVNULL)
                cfg = {"server_name": "localhost", "data_dir": str(root / "data"),
                       "bind_address": "127.0.0.1", "port": port,
                       "users": [{"localpart": u, "password_secret": ref}
                                 for u, ref in zip(("alice", "bob"), refs)],
                       "rooms": [{"key": "stable", "name": "Stable room", "creator": "alice",
                                  "invitees": ["bob"]}]}
                path = root / "config.json"
                path.write_text(json.dumps(cfg))

                first = json.loads(subprocess.run(command + ["setup", str(path)], check=True,
                                                  capture_output=True, text=True).stdout.splitlines()[-1])
                a = matrix.login(cfg, cfg["users"][0])
                b = matrix.login(cfg, cfg["users"][1])
                check(first["users"] == ["@alice:localhost", "@bob:localhost"], "two logins/IDs")
                stable = first["rooms"]["stable"]
                check(stable in matrix.request(cfg, "GET", "/_matrix/client/v3/joined_rooms", b)["joined_rooms"],
                      "provisioned room membership")

                # Explicit client create, invite, join and message exchange.
                room = matrix.request(cfg, "POST", "/_matrix/client/v3/createRoom", a,
                                      {"preset": "private_chat", "name": "Live E2E"})["room_id"]
                rid = urllib.parse.quote(room, safe="")
                matrix.request(cfg, "POST", "/_matrix/client/v3/rooms/%s/invite" % rid, a,
                               {"user_id": "@bob:localhost"})
                joined = matrix.request(cfg, "POST", "/_matrix/client/v3/join/%s" % rid, b, {})
                check(joined["room_id"] == room, "invite/join result")
                text_path = "/_matrix/client/v3/rooms/%s/send/m.room.message/%s" % (rid, transaction())
                text_event = matrix.request(cfg, "PUT", text_path, a,
                                            {"msgtype": "m.text", "body": "hello Bob"})["event_id"]
                reply_path = "/_matrix/client/v3/rooms/%s/send/m.room.message/%s" % (rid, transaction())
                reply_event = matrix.request(cfg, "PUT", reply_path, b,
                                             {"msgtype": "m.text", "body": "hello Alice"})["event_id"]
                image = b"\x89PNG\r\n\x1a\n" + b"matrix-live-image"
                upload_path = "/_matrix/media/v3/upload?filename=e2e.png"
                # Upload requires a content type and returns an mxc URI.
                req = urllib.request.Request(matrix.endpoint(cfg) + upload_path, image,
                    {"Authorization": "Bearer " + a, "Content-Type": "image/png"}, method="POST")
                with urllib.request.urlopen(req, timeout=15) as response:
                    mxc = json.load(response)["content_uri"]
                image_path = "/_matrix/client/v3/rooms/%s/send/m.room.message/%s" % (rid, transaction())
                image_event = matrix.request(cfg, "PUT", image_path, a,
                    {"msgtype": "m.image", "body": "e2e.png", "url": mxc,
                     "info": {"mimetype": "image/png", "size": len(image)}})["event_id"]

                second = json.loads(subprocess.run(command + ["setup", str(path)], check=True,
                                                   capture_output=True, text=True).stdout.splitlines()[-1])
                check(second["rooms"] == first["rooms"], "setup duplicated a stable room")
                subprocess.run(command + ["stop", str(path)], check=True)
                subprocess.run(command + ["start", str(path)], check=True)
                for _ in range(30):
                    try:
                        matrix.request(cfg, "GET", "/_matrix/client/versions")
                        break
                    except OSError:
                        time.sleep(1)
                else:
                    raise AssertionError("restart readiness")
                token = matrix.login(cfg, cfg["users"][1])
                for event_id, body in ((text_event, "hello Bob"), (reply_event, "hello Alice"),
                                       (image_event, "e2e.png")):
                    event = matrix.request(cfg, "GET", "/_matrix/client/v3/rooms/%s/event/%s" %
                                           (rid, urllib.parse.quote(event_id, safe="")), token)
                    check(event["content"]["body"] == body, "message lost after restart")
                server, media_id = mxc[6:].split("/", 1)
                download_urls = [
                    "/_matrix/client/v1/media/download/%s/%s" % (server, media_id),
                    "/_matrix/media/v3/download/%s/%s" % (server, media_id),
                ]
                for durl in download_urls:
                    try:
                        downloaded = matrix.request(cfg, "GET", durl, token, raw=True)
                        break
                    except Exception:
                        continue
                else:
                    raise AssertionError("media download failed")
                check(downloaded == image, "image bytes lost after restart")
                print("PASS: verified service, two logins, invite/join, text and image, idempotent setup, restart persistence")
        finally:
            if path and path.exists():
                subprocess.run(command + ["stop", str(path)], stdout=subprocess.DEVNULL,
                               stderr=subprocess.DEVNULL)
            for ref in refs:
                subprocess.run(["bashy", "secret", "rm", ref], stdout=subprocess.DEVNULL,
                               stderr=subprocess.DEVNULL)
            if sup:
                sup.terminate()
                try:
                    sup.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    sup.kill()
                    sup.wait()
            shutil.rmtree(sock_dir, ignore_errors=True)


if __name__ == "__main__":
    main()
