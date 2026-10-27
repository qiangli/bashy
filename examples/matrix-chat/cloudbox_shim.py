#!/usr/bin/env python3
"""Cloudbox account shim and client front for the matrix-chat homeserver.

Cloudbox S513.4a (cloudbox docs/matrix-chat.md) reaches this process as the
outpost app `matrix-chat`. Two kinds of traffic arrive on one listener:

* `POST /_cloudbox/v1/account` from Cloudbox over the tunnel, authenticated by
  the host's tunnel secret (Cloudbox MATRIX_TOKEN, injected from the vault).
  It creates, resets or deactivates an ordinary `@cb_<32 hex>` account.
* Native Matrix client traffic that Cloudbox's edge has already gated. Only
  the client, media and well-known roots are forwarded to tuwunel unchanged;
  the homeserver alone authenticates the client's Matrix token.

Accounts are created through a tuwunel application service whose exclusive
namespace is `@cb_[0-9a-f]{32}:<server_name>`. Its token cannot act outside
that namespace and is not a server admin. Registration stays disabled. The
current password of each account is kept in the host vault so a reset can
satisfy the homeserver's interactive auth; passwords and tokens are never
logged, and a password is returned only by create and reset.
"""
import argparse
import hmac
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import logging
import os
import re
import secrets
import subprocess
import sys
import threading
import urllib.parse

ACCOUNT_PATH = "/_cloudbox/v1/account"
HEALTH_PATH = "/_cloudbox/v1/health"
LOCALPART = re.compile(r"cb_[0-9a-f]{32}")
FORWARD_ROOTS = ("/_matrix/client/", "/_matrix/media/", "/.well-known/matrix/")
HOP_HEADERS = {"connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
               "te", "trailer", "trailers", "transfer-encoding", "upgrade", "host", "expect"}
# Outpost owns these; the homeserver must never see a stamped identity.
IDENTITY_HEADERS = {"x-periscope-user", "x-periscope-role", "remote-user", "remote-email",
                    "remote-name", "remote-groups", "x-outpost-identity-sig",
                    "x-outpost-identity-ts"}
MAX_BODY = 64 << 20
MAX_ACCOUNT_BODY = 16 << 10
# Vault names are at most 64 characters; a ref is <prefix>/cb_<32 hex>.
MAX_PREFIX = 64 - len("/cb_") - 32
TOKEN_ENV = "MATRIX_CHAT_CLOUDBOX_TOKEN"
AS_TOKEN_ENV = "MATRIX_CHAT_AS_TOKEN"

log = logging.getLogger("matrix-chat-cloudbox")


class ShimError(Exception):
    def __init__(self, status, errcode, message):
        super().__init__(message)
        self.status, self.errcode, self.message = status, errcode, message


class BashyVault:
    """Production credential store: the Bashy (Cloudbox) vault via `bashy secret`."""

    def __init__(self, runner="bashy"):
        self.runner = runner

    def get(self, ref):
        result = subprocess.run([self.runner, "secret", "show", ref], stdout=subprocess.PIPE,
                                stderr=subprocess.DEVNULL, text=True)
        return result.stdout.rstrip("\n") if result.returncode == 0 and result.stdout else None

    def set(self, ref, value):
        # The value travels on stdin, never in argv.
        subprocess.run([self.runner, "secret", "set", ref], input=value, text=True, check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def remove(self, ref):
        subprocess.run([self.runner, "secret", "rm", ref], stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL)


class Homeserver:
    """Minimal Matrix client for the appservice-scoped account operations."""

    def __init__(self, upstream, server_name, as_token, timeout=30):
        parsed = urllib.parse.urlsplit(upstream)
        if parsed.scheme != "http" or parsed.hostname not in ("127.0.0.1", "localhost", "::1"):
            raise ValueError("upstream must be a loopback http URL")
        self.host, self.port = parsed.hostname, parsed.port or 80
        self.base = parsed.path.rstrip("/")
        self.server_name = server_name
        self.as_token = as_token
        self.timeout = timeout

    def connection(self, timeout=None):
        return http.client.HTTPConnection(self.host, self.port, timeout=timeout or self.timeout)

    def call(self, method, path, token=None, body=None, query=None):
        url = self.base + path
        if query:
            url += "?" + urllib.parse.urlencode(query)
        headers = {}
        payload = None
        if token:
            headers["Authorization"] = "Bearer " + token
        if body is not None:
            payload = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        conn = self.connection()
        try:
            conn.request(method, url, payload, headers)
            response = conn.getresponse()
            raw = response.read()
        except OSError:
            raise ShimError(502, "M_UNKNOWN", "homeserver unreachable")
        finally:
            conn.close()
        try:
            data = json.loads(raw or b"{}")
        except ValueError:
            data = {}
        return response.status, data

    def user_id(self, localpart):
        return "@%s:%s" % (localpart, self.server_name)

    def exists(self, localpart):
        status, data = self.call("GET", "/_matrix/client/v3/profile/" +
                                 urllib.parse.quote(self.user_id(localpart), safe=""),
                                 self.as_token)
        if status == 404 and data.get("errcode") == "M_NOT_FOUND":
            return False
        if status == 200:
            return True
        raise ShimError(502, "M_UNKNOWN", "account lookup failed")

    def register(self, localpart, password):
        status, data = self.call("POST", "/_matrix/client/v3/register", self.as_token, {
            "type": "m.login.application_service", "username": localpart,
            "password": password, "inhibit_login": True})
        if status == 200:
            return True
        if data.get("errcode") == "M_USER_IN_USE":
            return False
        raise ShimError(502, "M_UNKNOWN", "account creation failed")

    def set_display_name(self, localpart, name):
        uid = self.user_id(localpart)
        status, _ = self.call("PUT", "/_matrix/client/v3/profile/%s/displayname" %
                              urllib.parse.quote(uid, safe=""), self.as_token,
                              {"displayname": name}, {"user_id": uid})
        if status != 200:
            raise ShimError(502, "M_UNKNOWN", "display name update failed")

    def session(self, localpart):
        """A short-lived device for the account, opened with the appservice token."""
        status, data = self.call("POST", "/_matrix/client/v3/login", self.as_token, {
            "type": "m.login.application_service",
            "identifier": {"type": "m.id.user", "user": localpart},
            "initial_device_display_name": "cloudbox provisioning"})
        if status != 200 or not data.get("access_token"):
            raise ShimError(502, "M_UNKNOWN", "account session failed")
        return data["access_token"]

    def logout(self, token):
        self.call("POST", "/_matrix/client/v3/logout", token, {})

    def with_password_auth(self, localpart, token, path, body, password):
        """Run a UIA-protected request, answering its m.login.password stage."""
        status, data = self.call("POST", path, token, body)
        if status == 200:
            return True
        session = data.get("session")
        if status != 401 or not session:
            raise ShimError(502, "M_UNKNOWN", "homeserver refused the account change")
        auth = {"type": "m.login.password", "session": session, "password": password,
                "identifier": {"type": "m.id.user", "user": localpart}}
        status, _ = self.call("POST", path, token, dict(body, auth=auth))
        return status == 200


class Provisioner:
    """The account actions behind POST /_cloudbox/v1/account."""

    def __init__(self, homeserver, vault, account_prefix):
        if not re.fullmatch(r"[A-Za-z0-9_./-]+", account_prefix) or \
                len(account_prefix.rstrip("/")) > MAX_PREFIX:
            raise ValueError("account secret prefix must be a vault reference of at most %d characters"
                             % MAX_PREFIX)
        self.hs = homeserver
        self.vault = vault
        self.prefix = account_prefix.rstrip("/")
        # One host-wide order for vault and homeserver changes.
        self.lock = threading.Lock()

    def ref(self, localpart):
        return "%s/%s" % (self.prefix, localpart)

    def handle(self, request):
        localpart = request.get("localpart")
        action = request.get("action")
        name = request.get("display_name")
        if not isinstance(localpart, str) or not LOCALPART.fullmatch(localpart):
            raise ShimError(400, "M_INVALID_PARAM", "localpart must be cb_ and 32 lower-case hex digits")
        if action not in ("ensure", "reset", "deactivate"):
            raise ShimError(400, "M_INVALID_PARAM", "action must be ensure, reset or deactivate")
        if name is not None and (not isinstance(name, str) or len(name) > 256):
            raise ShimError(400, "M_INVALID_PARAM", "invalid display_name")
        with self.lock:
            if action == "deactivate":
                return self.deactivate(localpart)
            exists = self.hs.exists(localpart)
            if not exists:
                result = self.create(localpart)
            elif action == "reset":
                result = self.reset(localpart)
            else:
                result = {"user_id": self.hs.user_id(localpart), "created": False}
            if name and name.strip():
                self.hs.set_display_name(localpart, name.strip())
            return result

    def create(self, localpart):
        password = secrets.token_urlsafe(24)
        ref = self.ref(localpart)
        # Vault first: an account must never exist with a password the host lost.
        previous = self.vault.get(ref)
        try:
            self.vault.set(ref, password)
        except (OSError, subprocess.CalledProcessError):
            raise ShimError(503, "M_UNKNOWN", "host vault unavailable")
        if not self.hs.register(localpart, password):
            # Lost a race with another creator: restore what the vault held.
            if previous is None:
                self.vault.remove(ref)
            else:
                self.vault.set(ref, previous)
            return {"user_id": self.hs.user_id(localpart), "created": False}
        return {"user_id": self.hs.user_id(localpart), "created": True, "password": password}

    def reset(self, localpart):
        ref = self.ref(localpart)
        current = self.vault.get(ref)
        if current is None:
            raise ShimError(409, "M_FORBIDDEN", "reset unavailable: no host credential for this account")
        password = secrets.token_urlsafe(24)
        token = self.hs.session(localpart)
        try:
            try:
                self.vault.set(ref, password)
            except (OSError, subprocess.CalledProcessError):
                raise ShimError(503, "M_UNKNOWN", "host vault unavailable")
            try:
                changed = self.hs.with_password_auth(localpart, token, "/_matrix/client/v3/account/password",
                                                     {"new_password": password, "logout_devices": True},
                                                     current)
            except ShimError:
                self.vault.set(ref, current)
                raise
        finally:
            self.hs.logout(token)
        if not changed:
            self.vault.set(ref, current)
            raise ShimError(409, "M_FORBIDDEN",
                            "reset unavailable: password was changed outside the host vault or account is deactivated")
        return {"user_id": self.hs.user_id(localpart), "created": False, "password": password}

    def deactivate(self, localpart):
        ref = self.ref(localpart)
        if not self.hs.exists(localpart):
            raise ShimError(404, "M_NOT_FOUND", "no such account")
        current = self.vault.get(ref)
        if current is None:
            raise ShimError(409, "M_FORBIDDEN", "deactivate unavailable: no host credential for this account")
        token = self.hs.session(localpart)
        try:
            done = self.hs.with_password_auth(localpart, token, "/_matrix/client/v3/account/deactivate",
                                              {"erase": False}, current)
        finally:
            # Deactivation already ended the session; a failed attempt must not leave it open.
            self.hs.logout(token)
        if not done:
            raise ShimError(409, "M_FORBIDDEN", "deactivate unavailable: host credential is stale")
        self.vault.remove(ref)
        return {"user_id": self.hs.user_id(localpart), "created": False}


def forwardable(raw_path):
    """True when a raw request path is inside a Matrix client root with no dot segments."""
    path = raw_path.split("?", 1)[0]
    decoded = urllib.parse.unquote(path)
    if "\\" in decoded or "//" in path:
        return False
    if any(seg in (".", "..") for seg in decoded.split("/")):
        return False
    return path.startswith(FORWARD_ROOTS) and decoded.startswith(FORWARD_ROOTS)


def make_handler(provisioner, homeserver, tunnel_token):
    expected = ("Bearer " + tunnel_token).encode()

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"
        server_version = "matrix-chat-cloudbox"
        sys_version = ""

        def log_message(self, fmt, *args):
            # Query strings can carry access tokens; log the path only.
            log.info("%s %s %s", self.command, self.path.split("?", 1)[0], args[1] if len(args) > 1 else "")

        def reply(self, status, body):
            payload = json.dumps(body).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(payload)))
            if status == 401:
                self.send_header("WWW-Authenticate", 'Bearer realm="cloudbox"')
            self.end_headers()
            self.wfile.write(payload)

        def error(self, status, errcode, message):
            self.reply(status, {"errcode": errcode, "error": message})

        def read_body(self, limit):
            if "chunked" in self.headers.get("Transfer-Encoding", "").lower():
                chunks, total = [], 0
                while True:
                    size = int(self.rfile.readline().split(b";", 1)[0].strip() or b"0", 16)
                    if size == 0:
                        while self.rfile.readline() not in (b"\r\n", b"\n", b""):
                            pass
                        return b"".join(chunks)
                    total += size
                    if total > limit:
                        raise ShimError(413, "M_TOO_LARGE", "request body too large")
                    chunks.append(self.rfile.read(size))
                    self.rfile.readline()
            length = int(self.headers.get("Content-Length") or 0)
            if length > limit:
                raise ShimError(413, "M_TOO_LARGE", "request body too large")
            return self.rfile.read(length) if length else b""

        def dispatch(self):
            path = self.path.split("?", 1)[0]
            try:
                if urllib.parse.unquote(path).startswith("/_cloudbox"):
                    self.cloudbox(path)
                elif forwardable(self.path):
                    self.forward()
                else:
                    self.error(404, "M_UNRECOGNIZED", "unrecognized request")
            except ShimError as exc:
                self.close_connection = True
                self.error(exc.status, exc.errcode, exc.message)
            except Exception as exc:  # never echo request data, which may hold secrets
                log.error("internal error on %s: %s", path, type(exc).__name__)
                self.close_connection = True
                self.error(500, "M_UNKNOWN", "internal error")

        def cloudbox(self, path):
            if path == HEALTH_PATH and self.command == "GET":
                self.reply(200, {"ok": True})
                return
            if path != ACCOUNT_PATH:
                self.error(404, "M_UNRECOGNIZED", "unrecognized request")
                return
            if self.command != "POST":
                self.error(405, "M_UNRECOGNIZED", "method not allowed")
                return
            supplied = self.headers.get("Authorization", "").encode()
            if not hmac.compare_digest(supplied, expected):
                self.close_connection = True
                self.error(401, "M_UNKNOWN_TOKEN", "invalid tunnel credential")
                return
            try:
                request = json.loads(self.read_body(MAX_ACCOUNT_BODY) or b"null")
            except ValueError:
                raise ShimError(400, "M_NOT_JSON", "invalid request body")
            if not isinstance(request, dict):
                raise ShimError(400, "M_NOT_JSON", "invalid request body")
            caller = self.headers.get("X-Periscope-User", "")
            try:
                result = provisioner.handle(request)
            except ShimError as exc:
                log.warning("account %s %s for %r: %s", request.get("action"), request.get("localpart"),
                            caller, exc.errcode)
                raise
            log.info("account %s %s for %r: created=%s", request.get("action"), request.get("localpart"),
                     caller, result.get("created"))
            self.reply(200, result)

        def forward(self):
            body = self.read_body(MAX_BODY)
            headers = {k: v for k, v in self.headers.items()
                       if k.lower() not in HOP_HEADERS and k.lower() not in IDENTITY_HEADERS
                       and k.lower() != "content-length"}
            if body or self.command in ("POST", "PUT", "PATCH"):
                headers["Content-Length"] = str(len(body))
            # Long-poll /sync waits up to the client's timeout (usually 30s).
            conn = homeserver.connection(timeout=120)
            started = False
            try:
                conn.request(self.command, homeserver.base + self.path, body or None, headers)
                upstream = conn.getresponse()
                started = True
                self.send_response(upstream.status, upstream.reason)
                for key, value in upstream.getheaders():
                    if key.lower() not in HOP_HEADERS and key.lower() != "content-length":
                        self.send_header(key, value)
                length = upstream.getheader("Content-Length")
                if length is not None:
                    self.send_header("Content-Length", length)
                    self.end_headers()
                    remaining = int(length)
                    while remaining > 0:
                        chunk = upstream.read(min(65536, remaining))
                        if not chunk:
                            break
                        self.wfile.write(chunk)
                        remaining -= len(chunk)
                else:
                    data = upstream.read()
                    self.send_header("Content-Length", str(len(data)))
                    self.end_headers()
                    self.wfile.write(data)
            except OSError:
                self.close_connection = True
                if not started:
                    self.error(502, "M_UNKNOWN", "homeserver unreachable")
            finally:
                conn.close()

        do_GET = do_POST = do_PUT = do_DELETE = do_PATCH = do_OPTIONS = do_HEAD = dispatch

    return Handler


def serve(argv=None, vault=None):
    parser = argparse.ArgumentParser(prog="cloudbox_shim.py serve")
    parser.add_argument("--listen", required=True, help="loopback host:port")
    parser.add_argument("--upstream", required=True, help="tuwunel base URL on loopback")
    parser.add_argument("--server-name", required=True)
    parser.add_argument("--account-prefix", required=True, help="vault prefix for account passwords")
    args = parser.parse_args(argv)
    tunnel = os.environ.get(TOKEN_ENV, "")
    as_token = os.environ.get(AS_TOKEN_ENV, "")
    if len(tunnel) < 16 or len(as_token) < 16:
        raise SystemExit("%s and %s must come from the vault (16+ characters)" % (TOKEN_ENV, AS_TOKEN_ENV))
    host, _, port = args.listen.rpartition(":")
    if host not in ("127.0.0.1", "localhost", "::1"):
        raise SystemExit("the shim listens on loopback only; outpost publishes it")
    homeserver = Homeserver(args.upstream, args.server_name, as_token)
    provisioner = Provisioner(homeserver, vault or BashyVault(), args.account_prefix)
    server = ThreadingHTTPServer((host, int(port)), make_handler(provisioner, homeserver, tunnel))
    server.daemon_threads = True
    log.info("listening on %s:%s for %s", host, port, args.server_name)
    server.serve_forever()


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    if len(sys.argv) < 2 or sys.argv[1] != "serve":
        raise SystemExit("Usage: cloudbox_shim.py serve --listen HOST:PORT --upstream URL "
                         "--server-name NAME --account-prefix REF")
    serve(sys.argv[2:])
