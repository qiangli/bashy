#!/usr/bin/env python3
"""Required live Matrix protocol E2E; fails on missing signed tool/service."""
import importlib.util
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
import urllib.parse

here = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("matrix_provision", here / "provision.py")
matrix = importlib.util.module_from_spec(spec)
spec.loader.exec_module(matrix)


def check(value, message):
    if not value:
        raise AssertionError(message)


def transaction():
    return "s513-" + str(time.time_ns())


def main():
    with tempfile.TemporaryDirectory(prefix="matrix-chat-e2e-") as temp:
        root = Path(temp)
        os.environ["BASHY_HOME"] = str(root / "bashy-home")
        os.environ["BASHY_SERVICES_DIR"] = str(root / "services")
        os.environ["BASHY_FLEET_DIR"] = str(root / "fleet")
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
        command = ["bashy", "--bashsharp", str(here / "matrix-chat.bsh")]
        try:
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
            import urllib.request
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
            downloaded = matrix.request(cfg, "GET", "/_matrix/media/v3/download/%s/%s" %
                                        (server, media_id), token, raw=True)
            check(downloaded == image, "image bytes lost after restart")
            print("PASS: verified service, two logins, invite/join, text and image, idempotent setup, restart persistence")
        finally:
            subprocess.run(command + ["stop", str(path)], stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL)
            for ref in refs:
                subprocess.run(["bashy", "secret", "rm", ref], stdout=subprocess.DEVNULL,
                               stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    main()
