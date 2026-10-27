#!/usr/bin/env python3
"""Local protocol gate for the Cloudbox account shim (S513.4a).

Runs Bashy's signed tuwunel and the real shim over loopback HTTP, then drives
the exact request Cloudbox's Provisioner sends. It proves the shim protocol,
the appservice namespace, three accounts, DM and group text+image through the
shim, room privacy, reset and deactivation.

TEST-ONLY by default: account passwords go to an in-memory synthetic vault
(MemoryVault) so this gate needs no Cloudbox vault token. With
MATRIX_CHAT_GATE_VAULT=bashy it uses the production BashyVault instead, under
a per-run prefix that is removed afterwards. Cloudbox's edge (share ACL,
revocation, HTTPS route) is not exercised here; cloudbox_e2e.py covers it.

The preflight is read-only: it never installs or downloads tuwunel. It reads
the binary path and this platform's signed asset sha256 from `bashy tool show
tuwunel`, and runs the installed file only if its digest matches. A missing or
mismatched binary fails the gate, and MATRIX_CHAT_TUWUNEL may only name that
same verified file. No containers are used.
"""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import secrets
import socket
import subprocess
import sys
import tempfile
import threading
import time
from http.server import ThreadingHTTPServer

here = Path(__file__).resolve().parent
sys.path.insert(0, str(here))
import mxclient  # noqa: E402

spec = importlib.util.spec_from_file_location("cloudbox_shim", here / "cloudbox_shim.py")
shim = importlib.util.module_from_spec(spec)
spec.loader.exec_module(shim)
spec = importlib.util.spec_from_file_location("matrix_provision", here / "provision.py")
provision = importlib.util.module_from_spec(spec)
spec.loader.exec_module(provision)


class MemoryVault:
    """Synthetic per-run secret adapter; test-only, never used by serve()."""

    def __init__(self):
        self.values = {}

    def get(self, ref):
        return self.values.get(ref)

    def set(self, ref, value):
        self.values[ref] = value

    def remove(self, ref):
        self.values.pop(ref, None)


def check(value, message):
    if not value:
        raise AssertionError(message)
    print("ok  ", message)


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def platform_key():
    """Bashy's asset key (os/arch) for this host."""
    arch = {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64",
            "arm64": "arm64"}.get(platform.machine().lower(), platform.machine().lower())
    return "%s/%s" % (sys.platform, arch)


def tool_metadata():
    """Bashy's tuwunel definition, read-only: `tool show` never installs or downloads."""
    try:
        result = subprocess.run(["bashy", "tool", "show", "tuwunel", "--json"],
                                stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=30)
    except (OSError, subprocess.TimeoutExpired):
        raise SystemExit("FAIL: bashy tool show tuwunel is unavailable")
    if result.returncode:
        raise SystemExit("FAIL: bashy tool show tuwunel failed")
    try:
        return json.loads(result.stdout)
    except ValueError:
        raise SystemExit("FAIL: bashy tool show tuwunel returned malformed metadata")


def sha256_file(path):
    digest = hashlib.sha256()
    with open(path, "rb") as fh:
        for block in iter(lambda: fh.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def tuwunel_binary(explicit=None, metadata=None):
    """The signed tuwunel to run, verified read-only; never installs or downloads.

    The path and the platform asset's sha256 come from Bashy's tool
    definition; the installed file must exist and match that digest. An
    override must be that same verified file. Anything else fails closed.
    """
    explicit = os.environ.get("MATRIX_CHAT_TUWUNEL") if explicit is None else explicit
    metadata = tool_metadata() if metadata is None else metadata
    try:
        cli = metadata["cli"]
        admitted = cli["binary"]
        expected = cli["managed"]["assets"][platform_key()]["sha256"]
    except (KeyError, TypeError):
        raise SystemExit("FAIL: no signed tuwunel asset for %s in Bashy's tool metadata"
                         % platform_key())
    if not isinstance(admitted, str) or not isinstance(expected, str) or len(expected) != 64:
        raise SystemExit("FAIL: Bashy's tuwunel metadata is malformed")
    for prefix in ("$HOME/", "${HOME}/"):
        if admitted.startswith(prefix):
            admitted = str(Path.home() / admitted[len(prefix):])
    if "$" in admitted or not os.path.isabs(admitted):
        # A bare name means Bashy has no installed version to point at.
        raise SystemExit("FAIL: signed tuwunel is not installed (no absolute path in Bashy's metadata)")
    if not Path(admitted).is_file():
        raise SystemExit("FAIL: signed tuwunel is not installed; run `bashy tool install tuwunel` first")
    if sha256_file(admitted) != expected.lower():
        raise SystemExit("FAIL: installed tuwunel does not match the signed %s asset digest"
                         % platform_key())
    if explicit:
        try:
            same = os.path.samefile(explicit, admitted)
        except OSError:
            same = False
        if not same:
            raise SystemExit("FAIL: MATRIX_CHAT_TUWUNEL is not the verified signed tuwunel")
    return admitted


class Accounts:
    """Every localpart this run mints, so cleanup can reach each vault entry."""

    def __init__(self):
        self.used = []

    def __call__(self):
        self.used.append("cb_" + secrets.token_hex(16))
        return self.used[-1]


def cleanup(prov, vault, accounts):
    """Remove each run account's credential through Provisioner.ref; return any left behind."""
    left = []
    for user in accounts.used:
        ref = prov.ref(user)
        try:
            vault.remove(ref)
        except Exception as exc:
            print("cleanup: %s not removed: %s" % (ref, type(exc).__name__))
        try:
            if vault.get(ref) is not None:
                left.append(ref)
        except Exception:
            left.append(ref)
    return left


def main():
    binary = tuwunel_binary()
    server_name = "cloudbox-gate.test"
    as_token, hs_token, tunnel = (secrets.token_urlsafe(32) for _ in range(3))
    with tempfile.TemporaryDirectory(prefix="matrix-cloudbox-gate-") as temp:
        root = Path(temp)
        hs_port, shim_port = free_port(), free_port()
        cfg = {"server_name": server_name, "port": hs_port}
        registration = root / "appservice.toml"
        registration.write_text(provision.appservice_toml(cfg))
        env = dict(os.environ, TUWUNEL_CONFIG=str(registration), TUWUNEL_SERVER_NAME=server_name,
                   TUWUNEL_DATABASE_PATH=str(root / "db"), TUWUNEL_ADDRESS="127.0.0.1",
                   TUWUNEL_PORT=str(hs_port), TUWUNEL_ALLOW_REGISTRATION="false",
                   TUWUNEL_ALLOW_FEDERATION="false", TUWUNEL_NEW_USER_DISPLAYNAME_SUFFIX="",
                   TUWUNEL_APPSERVICE__CLOUDBOX__AS_TOKEN=as_token,
                   TUWUNEL_APPSERVICE__CLOUDBOX__HS_TOKEN=hs_token)
        log = open(root / "tuwunel.log", "wb")
        proc = subprocess.Popen([binary], env=env, stdout=log, stderr=subprocess.STDOUT)
        hs = mxclient.Client("http://127.0.0.1:%d" % hs_port)
        httpd = prov = vault = None
        accounts = Accounts()
        try:
            for _ in range(60):
                try:
                    if hs.call("GET", "/_matrix/client/versions")[0] == 200:
                        break
                except OSError:
                    pass
                time.sleep(0.5)
            else:
                raise SystemExit("FAIL: tuwunel did not become ready")
            if os.environ.get("MATRIX_CHAT_GATE_VAULT") == "bashy":
                vault, prefix = shim.BashyVault(), "matrix-chat/g-%s" % secrets.token_hex(4)
                print("vault: production BashyVault, prefix", prefix)
            else:
                vault, prefix = MemoryVault(), "gate/accounts"
                print("vault: synthetic in-memory test adapter")
            homeserver = shim.Homeserver("http://127.0.0.1:%d" % hs_port, server_name, as_token)
            prov = shim.Provisioner(homeserver, vault, prefix)
            httpd = ThreadingHTTPServer(("127.0.0.1", shim_port), shim.make_handler(prov, homeserver, tunnel))
            httpd.daemon_threads = True
            threading.Thread(target=httpd.serve_forever, daemon=True).start()
            # Cloudbox reaches the shim at <outpost>/app/matrix-chat; outpost strips the mount.
            front = mxclient.Client("http://127.0.0.1:%d" % shim_port)
            cloudbox = mxclient.Client(front.base, tunnel, {"X-Periscope-User": "gate@example.test"})
            run(hs, front, cloudbox, as_token, vault, prov, accounts, server_name)
        finally:
            if httpd:
                httpd.shutdown()
            # Runs on every exit, including a failed check or interrupt mid-run,
            # after the shim stops so no in-flight request rewrites a credential.
            left = cleanup(prov, vault, accounts) if prov else []
            proc.terminate()
            try:
                proc.wait(20)
            except subprocess.TimeoutExpired:
                proc.kill()
            log.close()
            if left:
                raise SystemExit("FAIL: test credentials left in the vault: %s" % ", ".join(left))
    print("PASS: Cloudbox shim protocol gate (signed tuwunel, real shim, %s vault)" %
          ("production" if os.environ.get("MATRIX_CHAT_GATE_VAULT") == "bashy" else "synthetic test"))


def run(hs, front, cloudbox, as_token, vault, prov, localpart, server_name):
    account = shim.ACCOUNT_PATH
    users = [localpart() for _ in range(4)]
    one, two, three, outsider = users

    # Gate: the tunnel secret is required and is never forwarded upstream.
    status, _ = front.call("POST", account, {"localpart": one, "action": "ensure"})
    check(status == 401, "account request without tunnel secret is refused")
    status, _ = front.call("POST", account, {"localpart": one, "action": "ensure"}, token="x" * 40)
    check(status == 401, "account request with a wrong secret is refused")
    status, _ = front.call("POST", account, {"localpart": one, "action": "ensure"}, token=as_token)
    check(status == 401, "the appservice token is not a tunnel credential")
    for bad in ({"localpart": "alice", "action": "ensure"}, {"localpart": one, "action": "admin"},
                {"localpart": "cb_" + "A" * 32, "action": "ensure"}):
        check(cloudbox.call("POST", account, bad)[0] == 400, "invalid request refused: %s" % bad)
    for path in ("/_cloudbox/v1/other", "/_matrix/federation/v1/version", "/_synapse/admin/v1/users",
                 "/_matrix/client/../../_cloudbox/v1/account", "/_matrix/client/%2e%2e/x",
                 "/_tuwunel/oidc/jwks"):
        status, _ = cloudbox.call("GET", path)
        check(status == 404, "not forwarded: " + path)
    check(front.call("GET", "/_matrix/client/versions")[0] == 200, "client discovery forwarded")

    # Opt-in: password exactly once per account.
    passwords = {}
    names = {one: "User One", two: "User Two", three: "User Three", outsider: "Outsider"}
    for user in users:
        status, data = cloudbox.call("POST", account, {"localpart": user, "display_name": names[user],
                                                       "action": "ensure"})
        check(status == 200 and data.get("created") is True and data.get("password")
              and data["user_id"] == "@%s:%s" % (user, server_name),
              "ensure creates %s (HTTP %s %s)" % (user, status, data.get("errcode", "")))
        passwords[user] = data["password"]
        status, data = cloudbox.call("POST", account, {"localpart": user, "action": "ensure"})
        check(status == 200 and data.get("created") is False and "password" not in data,
              "second ensure returns no password for " + user)
    check(vault.get(prov.ref(one)) == passwords[one], "host vault holds the credential")
    fresh = localpart()
    status, data = cloudbox.call("POST", account, {"localpart": fresh, "action": "reset"})
    check(status == 200 and data.get("created") is True and data.get("password"),
          "reset of a missing account creates it (Cloudbox live E2E opens with reset)")
    status, data = hs.call("GET", "/_matrix/client/v3/profile/%s/displayname" %
                           mxclient.q("@%s:%s" % (one, server_name)))
    check(status == 200 and data.get("displayname") == "User One", "display name set from Cloudbox")

    # Registration stays closed; the appservice is confined to its namespace.
    status, data = hs.call("POST", "/_matrix/client/v3/register", {"username": "cb_" + "f" * 32,
                                                                   "password": secrets.token_urlsafe(16)})
    check(status in (401, 403) and data.get("errcode") != "M_USER_IN_USE", "open registration is disabled")
    status, data = hs.call("POST", "/_matrix/client/v3/register",
                           {"type": "m.login.application_service", "username": "mallory",
                            "password": secrets.token_urlsafe(16)}, token=as_token)
    check(status != 200, "appservice cannot create accounts outside @cb_*")

    tokens = {}
    for user in users:
        status, data = mxclient.login(front, user, passwords[user])
        check(status == 200, "password login through the shim for " + user)
        tokens[user] = data["access_token"]
    mx = {u: "@%s:%s" % (u, server_name) for u in users}
    image = b"\x89PNG\r\n\x1a\n" + secrets.token_bytes(64)

    # DM one <-> two: text and image.
    status, data = front.call("POST", "/_matrix/client/v3/createRoom",
                              {"preset": "trusted_private_chat", "is_direct": True, "invite": [mx[two]]},
                              token=tokens[one])
    check(status == 200, "DM created")
    dm = data["room_id"]
    check(front.call("POST", "/_matrix/client/v3/join/" + mxclient.q(dm), {}, token=tokens[two])[0] == 200,
          "DM invite accepted")
    check(mxclient.send_text(front, tokens[one], dm, "dm hello")[0] == 200, "DM text sent")
    status, _, dm_mxc = mxclient.send_image(front, tokens[two], dm, image)
    check(status == 200, "DM image sent")
    status, got = mxclient.bodies(front, tokens[two], dm)
    check("dm hello" in got and "cloudbox-e2e.png" in got, "DM text and image delivered")
    status, data = mxclient.download(front, tokens[one], dm_mxc)
    check(status == 200 and data == image, "DM image bytes round-trip")
    status, _ = front.call("GET", "/_matrix/client/v3/rooms/%s/messages?dir=b" % mxclient.q(dm),
                           token=tokens[three])
    check(status == 403, "uninvolved user cannot read the DM")
    status, _ = front.call("POST", "/_matrix/client/v3/join/" + mxclient.q(dm), {}, token=tokens[outsider])
    check(status == 403, "uninvolved user cannot join the DM")

    # Private group of three: text and image.
    status, data = front.call("POST", "/_matrix/client/v3/createRoom",
                              {"preset": "private_chat", "name": "Gate group", "invite": [mx[two], mx[three]]},
                              token=tokens[one])
    check(status == 200, "group created")
    group = data["room_id"]
    for user in (two, three):
        check(front.call("POST", "/_matrix/client/v3/join/" + mxclient.q(group), {},
                         token=tokens[user])[0] == 200, "group invite accepted by " + user)
    check(mxclient.send_text(front, tokens[three], group, "group hello")[0] == 200, "group text sent")
    status, _, group_mxc = mxclient.send_image(front, tokens[two], group, image)
    check(status == 200, "group image sent")
    for user in (one, two, three):
        status, got = mxclient.bodies(front, tokens[user], group)
        check("group hello" in got and "cloudbox-e2e.png" in got, "group messages visible to " + user)
    check(front.call("POST", "/_matrix/client/v3/join/" + mxclient.q(group), {},
                     token=tokens[outsider])[0] == 403, "uninvited user cannot join the group")

    # Reset rotates the password and ends existing sessions.
    status, data = cloudbox.call("POST", account, {"localpart": two, "action": "reset"})
    check(status == 200 and data.get("created") is False and data.get("password")
          and data["password"] != passwords[two], "reset issues a new password")
    check(mxclient.login(front, two, passwords[two])[0] == 403, "old password no longer works")
    check(front.call("GET", "/_matrix/client/v3/account/whoami", token=tokens[two])[0] == 401,
          "reset logged out existing sessions")
    status, data = mxclient.login(front, two, data["password"])
    check(status == 200, "new password works")
    passwords[two] = None

    # Deactivation is final: no login, no reset, credential removed.
    status, data = cloudbox.call("POST", account, {"localpart": outsider, "action": "deactivate"})
    check(status == 200 and data.get("created") is False and "password" not in data, "deactivate")
    check(mxclient.login(front, outsider, passwords[outsider])[0] in (401, 403), "deactivated login refused")
    check(vault.get(prov.ref(outsider)) is None, "deactivated credential removed")
    status, _ = cloudbox.call("POST", account, {"localpart": outsider, "action": "reset"})
    check(status == 409, "deactivated account cannot be reset")
    status, _ = cloudbox.call("POST", account, {"localpart": localpart(), "action": "deactivate"})
    check(status == 404, "deactivating an unknown account is 404")


if __name__ == "__main__":
    main()
