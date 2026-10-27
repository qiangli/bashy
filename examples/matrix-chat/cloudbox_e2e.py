#!/usr/bin/env python3
"""Live Cloudbox + Matrix E2E for S513.4a (Bashy side of cloudbox 1b1af90).

Drives a real Cloudbox over HTTPS whose CHAT_MATRIX_HOST runs
`bashy matrix-chat setup` + `bashy matrix-chat cloudbox` with the real host
vault. Every Matrix call uses the homeserver URL Cloudbox hands out
(https://<cloudbox>/matrix/h/<host>/app/matrix-chat), so traffic crosses the
Cloudbox edge, the tunnel, outpost, the shim and tuwunel.

Covers: three distinct Cloudbox users with chat:matrix opt in and see their
password exactly once; DM and private group text+image; an uninvolved user is
denied (onboarding, login, room reads); revoking a share blocks the user's
live Matrix session, login and onboarding; the share is restored afterwards.

Prerequisites (all required; absence is reported as BLOCKED, exit 2, never PASS):

  CHAT_MATRIX_E2E_CLOUDBOX       https origin of the test Cloudbox
  CHAT_MATRIX_E2E_USER1_TOKEN    Bearer chat:matrix, user with a matrix-chat share
  CHAT_MATRIX_E2E_USER2_TOKEN    Bearer chat:matrix, second user with a share
  CHAT_MATRIX_E2E_USER3_TOKEN    Bearer chat:matrix, third user with a share (revoked mid-run)
  CHAT_MATRIX_E2E_USER3_EMAIL    user 3's Cloudbox email (to find the share row)
  CHAT_MATRIX_E2E_OUTSIDER_TOKEN Bearer chat:matrix, user with no share
  CHAT_MATRIX_E2E_OWNER_TOKEN    Bearer accepted by /api/hosts/<host>/shares (host owner)
  CHAT_MATRIX_E2E_ALLOW_HTTP=1   optional, only for a loopback test Cloudbox

Run only against a disposable test deployment: it resets user passwords
(only when an account pre-exists) and deletes/recreates user 3's share.
"""
import os
from pathlib import Path
import secrets
import sys
import urllib.parse

sys.path.insert(0, str(Path(__file__).resolve().parent))
import mxclient  # noqa: E402

REQUIRED = ["CHAT_MATRIX_E2E_CLOUDBOX", "CHAT_MATRIX_E2E_USER1_TOKEN", "CHAT_MATRIX_E2E_USER2_TOKEN",
            "CHAT_MATRIX_E2E_USER3_TOKEN", "CHAT_MATRIX_E2E_USER3_EMAIL",
            "CHAT_MATRIX_E2E_OUTSIDER_TOKEN", "CHAT_MATRIX_E2E_OWNER_TOKEN"]
INFO = "/api/v1/chat/matrix"
ACCOUNT = "/api/v1/chat/matrix/account"


def check(value, message):
    if not value:
        raise AssertionError("FAIL: " + message)
    print("ok  ", message)


def opt_in(cb, token, label):
    """Opt in once; the password is shown exactly once per issue."""
    status, data = cb.call("POST", ACCOUNT, {"action": "ensure"}, token=token)
    check(status == 200 and data.get("user_id"), "%s ensure answered" % label)
    if data.get("created"):
        check(bool(data.get("password")), "%s new account returns its password" % label)
    else:
        check(not data.get("password"), "%s existing account: ensure returns no password" % label)
        # A rerun on the same deployment: reset is the documented way to get a new one.
        status, data = cb.call("POST", ACCOUNT, {"action": "reset"}, token=token)
        check(status == 200 and data.get("password"), "%s reset issues a password" % label)
    password = data["password"]
    status, again = cb.call("POST", ACCOUNT, {"action": "ensure"}, token=token)
    check(status == 200 and again.get("created") is False and not again.get("password"),
          "%s password is not shown again" % label)
    return data["user_id"], password


def main():
    missing = [k for k in REQUIRED if not os.environ.get(k)]
    if missing:
        print("BLOCKED: live Cloudbox HTTPS E2E prerequisites absent: " + ", ".join(missing))
        print("No PASS is claimed. See cloudbox_e2e.py docstring and README.md.")
        return 2
    origin = os.environ["CHAT_MATRIX_E2E_CLOUDBOX"].rstrip("/")
    allow_http = os.environ.get("CHAT_MATRIX_E2E_ALLOW_HTTP") == "1"
    if not origin.startswith("https://") and not allow_http:
        print("BLOCKED: CHAT_MATRIX_E2E_CLOUDBOX must be https")
        return 2
    users = [os.environ["CHAT_MATRIX_E2E_USER%d_TOKEN" % i] for i in (1, 2, 3)]
    outsider = os.environ["CHAT_MATRIX_E2E_OUTSIDER_TOKEN"]
    owner = os.environ["CHAT_MATRIX_E2E_OWNER_TOKEN"]
    email3 = os.environ["CHAT_MATRIX_E2E_USER3_EMAIL"].strip().lower()
    cb = mxclient.Client(origin)

    infos = []
    for i, token in enumerate(users, 1):
        status, info = cb.call("GET", INFO, token=token)
        check(status == 200 and info.get("enabled") and info.get("allowed"), "user%d may use chat" % i)
        infos.append(info)
    check(len({i["user_id"] for i in infos}) == 3, "three distinct mapped Matrix IDs")
    base = infos[0]["homeserver_url"]
    check(base.startswith("https://") or allow_http, "homeserver URL is the HTTPS Cloudbox route")
    check(all(i["homeserver_url"] == base for i in infos), "one homeserver for the deployment")
    host = urllib.parse.urlsplit(base).path.split("/matrix/h/", 1)[1].split("/", 1)[0]
    status, info = cb.call("GET", INFO, token=outsider)
    check(status == 200 and not info.get("allowed") and not info.get("homeserver_url"),
          "outsider is not offered chat")
    status, _ = cb.call("POST", ACCOUNT, {"action": "ensure"}, token=outsider)
    check(status == 403, "outsider cannot opt in")

    accounts = [opt_in(cb, token, "user%d" % i) for i, token in enumerate(users, 1)]
    mx = mxclient.Client(base)
    check(mx.call("GET", "/_matrix/client/versions")[0] == 200, "/versions through the edge")
    tokens = []
    for i, (user_id, password) in enumerate(accounts, 1):
        status, data = mxclient.login(mx, user_id, password)
        check(status == 200 and data.get("user_id") == user_id, "user%d logs in over HTTPS" % i)
        tokens.append(data["access_token"])
    outsider_id = info.get("user_id") or "@cb_%s:x" % ("0" * 32)
    status, _ = mxclient.login(mx, outsider_id, secrets.token_urlsafe(16))
    check(status == 403, "outsider login is refused at the edge")

    image = b"\x89PNG\r\n\x1a\n" + secrets.token_bytes(128)
    ids = [a[0] for a in accounts]

    status, data = mx.call("POST", "/_matrix/client/v3/createRoom",
                           {"preset": "trusted_private_chat", "is_direct": True, "invite": [ids[1]]},
                           token=tokens[0])
    check(status == 200, "DM created")
    dm = data["room_id"]
    check(mx.call("POST", "/_matrix/client/v3/join/" + mxclient.q(dm), {}, token=tokens[1])[0] == 200,
          "DM joined")
    check(mxclient.send_text(mx, tokens[0], dm, "dm over cloudbox")[0] == 200, "DM text")
    status, _, dm_mxc = mxclient.send_image(mx, tokens[1], dm, image)
    check(status == 200, "DM image")
    check(set(mxclient.bodies(mx, tokens[1], dm)[1]) >= {"dm over cloudbox", "cloudbox-e2e.png"},
          "DM text and image delivered")
    status, got = mxclient.download(mx, tokens[0], dm_mxc)
    check(status == 200 and got == image, "DM image bytes over HTTPS")
    check(mx.call("GET", "/_matrix/client/v3/rooms/%s/messages?dir=b" % mxclient.q(dm),
                  token=tokens[2])[0] == 403, "uninvolved user3 cannot read the DM")

    status, data = mx.call("POST", "/_matrix/client/v3/createRoom",
                           {"preset": "private_chat", "name": "S513 E2E", "invite": ids[1:]}, token=tokens[0])
    check(status == 200, "group created")
    group = data["room_id"]
    for i in (1, 2):
        check(mx.call("POST", "/_matrix/client/v3/join/" + mxclient.q(group), {}, token=tokens[i])[0] == 200,
              "user%d joined the group" % (i + 1))
    check(mxclient.send_text(mx, tokens[2], group, "group over cloudbox")[0] == 200, "group text")
    status, _, group_mxc = mxclient.send_image(mx, tokens[1], group, image)
    check(status == 200, "group image")
    for i in range(3):
        check(set(mxclient.bodies(mx, tokens[i], group)[1]) >= {"group over cloudbox", "cloudbox-e2e.png"},
              "user%d sees group text and image" % (i + 1))
    status, got = mxclient.download(mx, tokens[2], group_mxc)
    check(status == 200 and got == image, "group image bytes over HTTPS")

    # Revocation: the owner deletes user3's share; the next request is refused.
    shares_path = "/api/hosts/%s/shares" % mxclient.q(host)
    status, data = cb.call("GET", shares_path, token=owner)
    check(status == 200, "owner lists host shares")
    rows = [r for r in data.get("shares", []) if r.get("app") == "matrix-chat"
            and r.get("member_email", "").lower() == email3]
    check(len(rows) == 1, "user3 holds exactly one matrix-chat share")
    restored = False
    try:
        status, _ = cb.call("DELETE", "%s/%s" % (shares_path, rows[0]["id"]), token=owner)
        check(status in (200, 204), "owner revokes user3's share")
        status, data = mx.call("GET", "/_matrix/client/v3/sync?timeout=0", token=tokens[2])
        check(status == 403 and data.get("errcode") == "M_FORBIDDEN", "revoked user's live session is refused")
        check(mxclient.send_text(mx, tokens[2], group, "after revoke")[0] == 403, "revoked user cannot post")
        check(mxclient.login(mx, ids[2], accounts[2][1])[0] == 403, "revoked user cannot log in")
        check(cb.call("POST", ACCOUNT, {"action": "reset"}, token=users[2])[0] == 403,
              "revoked user cannot obtain a password")
        check("after revoke" not in mxclient.bodies(mx, tokens[0], group)[1], "nothing posted after revocation")
        check(mx.call("GET", "/_matrix/client/v3/sync?timeout=0", token=tokens[0])[0] == 200,
              "other users are unaffected")
    finally:
        status, _ = cb.call("POST", shares_path, {"member_email": email3, "apps": ["matrix-chat"]}, token=owner)
        restored = status in (200, 201)
    check(restored, "user3's share restored for the next run")
    print("PASS: live Cloudbox HTTPS Matrix E2E (3 users, DM+group text/image, outsider denied, revocation)")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except AssertionError as exc:
        print(exc)
        sys.exit(1)
