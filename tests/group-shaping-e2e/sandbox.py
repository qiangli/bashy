"""Process supervisor and synthetic clients. Run only inside the disposable image."""
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError
from urllib.parse import urlsplit
from urllib.request import Request, urlopen

APP = "site-shaping"
MOUNT = "/app/" + APP
FRONT = "http://127.0.0.1:18880"
ORIGIN = "http://127.0.0.1:18881"
ADMIN = "127.0.0.1:18882"
PAGES = 18883
ENGINE = 18884
OWNER, A, C, NON_SUPPORT, ACCOUNT = [
    f"{name}@example.invalid" for name in ("o", "a", "c", "member", "account")]
TOKEN = secrets.token_hex(24)
GRANTED = threading.Event()
HOME = Path.home()
CONFIG = HOME / ".config/matrix/agent.json"
WEBSITE = ["bashy", "--bashsharp", "/website/website.bsh"]
BINDING = "codex:gpt6-sol"


class Fixture(BaseHTTPRequestHandler):
    """The grant-copy endpoint and the public static site fronted by website.bsh."""

    def do_GET(self):
        if self.path == "/api/v1/hosts/self/grants":
            if self.headers.get("Authorization") != "Bearer " + TOKEN:
                self.send_error(401)
                return
            data = {"host": "test-host", "owner": OWNER, "revision": 1,
                    "members": [{"email": A, "circle": "company"},
                                {"email": C, "circle": "contractor"},
                                {"email": NON_SUPPORT, "circle": "company"}],
                    "groups": {"support": [A, C]}, "apps": {APP: [{"groups": ["support"]}]}}
            GRANTED.set()
            mime, body = "application/json", json.dumps(data)
        elif self.path == "/index.json":
            mime, body = "application/json", json.dumps(
                {"path": "/", "title": "Support desk", "revision": "fixture-v1", "sections": []})
        elif self.path == "/index.md":
            mime, body = "text/markdown", "# Support desk\n"
        elif self.path == "/":
            mime, body = "text/html", "<html><body>Support desk</body></html>"
        else:
            self.send_error(404)
            return
        raw = body.encode()
        self.send_response(200)
        self.send_header("Content-Type", mime)
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)


def check(condition, message):
    if not condition:
        raise AssertionError(message)
    print(f"ok - {message}", flush=True)


def run(*args):
    print("+", *args, flush=True)
    subprocess.run(args, check=True, timeout=150)


def ready(process, port):
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f"{process.args[0]} exited {process.returncode} before readiness")
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=.5):
                return
        except OSError:
            time.sleep(.1)
    raise TimeoutError(f"listener {port} did not start")


def headers(user):
    result = {"X-Forwarded-Prefix": MOUNT}
    if user:
        result["X-Periscope-User"] = user
    return result


def call(user, tool, args):
    body = {"jsonrpc": "2.0", "id": 1, "method": "tools/call",
            "params": {"name": tool, "arguments": args}}
    hdr = dict(headers(user), Accept="application/json, text/event-stream",
               **{"Content-Type": "application/json", "MCP-Protocol-Version": "2025-06-18"})
    req = Request(FRONT + MOUNT + "/mcp", json.dumps(body).encode(), hdr)
    try:
        with urlopen(req, timeout=180) as res:
            raw = res.read().decode()
            print(user or "anonymous", tool, res.status, raw, flush=True)
            if res.headers.get_content_type() == "text/event-stream":
                raw, = [line[6:] for line in raw.splitlines() if line.startswith("data: ")]
            reply = json.loads(raw)
            return reply["result"]
    except HTTPError as exc:
        print(user or "anonymous", tool, exc.code, exc.read().decode(), flush=True)
        if exc.code == 401 and exc.headers.get("WWW-Authenticate", "").endswith(
                MOUNT + '/.well-known/oauth-protected-resource"'):
            return {"challenge": True}
        raise


def success(user, tool, args):
    result = call(user, tool, args)
    if result.get("isError") or "structuredContent" not in result:
        raise AssertionError(f"{tool}: {result}")
    return result["structuredContent"]


def shape(verb, args):
    result = success(OWNER, "shape_" + verb, args)
    check((result["principal"], result["app"], result["verb"]) == (OWNER, APP, verb),
          f"{verb} attributed to owner on the mounted app")
    return result["result"]


def member_refused(member, tool, result):
    reason = f"refused: caller {member} (tier member) is not authorized for {tool}"
    check(result.get("isError") and reason in result["content"][0]["text"],
          f"{member} refused {tool} as a member")


def active():
    status = json.loads(subprocess.check_output(["bashy", "shape", "status", APP,
                                                 "--user", OWNER, "--json"]))
    return status["active"]


def scenario():
    wish = {"request": "Publish the support greeting", "agent": BINDING}
    with urlopen(Request(FRONT + MOUNT + "/_shaping/api/me", headers=headers(OWNER)),
                 timeout=10) as response:
        panel = json.load(response)
    check(panel["tier"] == "owner" and panel["title"] == "Original support greeting",
          "served website panel recognizes the owner and shows the base title")
    for member in (A, C):
        page = success(member, "read_page", {"path_or_url": "/", "format": "md"})
        check(page["content"].strip() == "# Support desk" and page["revision"] == "fixture-v1",
              f"{member} read the served page through MCP")
        result = call(member, "shape_wish", wish)
        member_refused(member, "shape_wish", result)
    for user in (NON_SUPPORT, ACCOUNT, None):
        check(call(user, "shape_wish", wish).get("challenge"),
              f"{user or 'anonymous'} refused by the mounted app grant gate")
    candidate = shape("wish", wish)
    cid, rev = candidate["id"], candidate["rev"]
    check(cid and rev and candidate["base_rev"] != rev and candidate["status"] == "ready",
          "wish produced a new ready revision")
    check((HOME / "stub-events").read_text().splitlines() == ["edit"],
          "exactly one fixed edit ran through the delegate")
    diff = subprocess.check_output(["bashy", "shape", "diff", APP, cid, "--user", OWNER], text=True)
    print(diff, flush=True)
    check('-  "title": "Original support greeting"' in diff and
          '+  "title": "Approved support greeting"' in diff, "candidate contains the fixed edit")
    verified = shape("verify", {"id": cid})
    check(verified["id"] == cid and verified["rev"] == rev and verified["status"] == "verified"
          and verified["verdict"]["pass"], "trusted verifier passed the candidate revision")
    evidence = json.loads(Path(verified["verdict"]["evidence"]).read_text())
    print("verifier evidence", json.dumps(evidence), flush=True)
    check(evidence["binding"]["candidate"] == rev and evidence["binding"]["principal"] == OWNER and
          any(c["name"] == "checks/build.bsh" and c["status"] == "pass" for c in evidence["checks"]),
          "sealed verifier evidence records the website check on the owner's revision")
    for member in (A, C):
        for tool in ("shape_verify", "shape_preview", "shape_approve"):
            member_refused(member, tool, call(member, tool, {"id": cid}))
    preview = shape("preview", {"id": cid})
    check(preview["candidate"]["rev"] == rev and
          urlsplit(preview["url"]).path == f"/shape/{APP}/preview/{cid}/",
          "MCP returned the isolated preview for the verified candidate")
    check(active() == candidate["base_rev"], "preview left the base revision active")
    approved = shape("approve", {"id": cid})
    check(approved["id"] == cid and approved["rev"] == rev and approved["status"] == "approved",
          "owner approved the same revision")
    check(active() == rev, "owner's active revision matches approval")


def start_outpost():
    proc = subprocess.Popen(["outpost", "start"])
    ready(proc, 18880)
    return proc


def main():
    os.environ.update(PATH="/fixture:/tools:/go/bin:/python/bin:/usr/bin:/bin",
                      BASHY_HOME=str(HOME / ".bashy"), BASHY_HINTS="off",
                      GIT_TERMINAL_PROMPT="0", USER="bashy", LOGNAME="bashy",
                      GOMAXPROCS="4", GOTOOLCHAIN="local", GOPROXY="off", GOWORK="off",
                      OUTPOST_ADMIN_ADDR=ADMIN, OUTPOST_INGRESS_TRUST_LOOPBACK="1")
    # delegate's environment allowlist carries HOME and PATH. Keep its real
    # prebuilt adapter at the default cache path used by the embedded recipe.
    cache = HOME / ".cache/bashy/adapters/yoke"
    cache.mkdir(parents=True, exist_ok=True)
    shutil.copy("/tools/delegate", cache / "delegate")
    stub = shutil.which("codex")
    check(stub == "/fixture/codex", "fixed-edit Codex CLI found on sandbox PATH")
    run("bashy", "tools", "set", "codex", "--set", "cli.binary=" + stub)
    invocation = json.loads(subprocess.check_output(
        ["bashy", "delegate", "--dry-run", "--json", "--agent", BINDING, "binding preflight"],
        env=dict(os.environ, BASHY_DELEGATE_CALLER="genie", BASHY_DELEGATE_ALLOW=BINDING)))
    check(invocation["agent"] == stub, f"{BINDING} resolves to {stub}")
    run("git", "config", "--global", "user.name", "Fixture")
    run("git", "config", "--global", "user.email", OWNER)
    CONFIG.parent.mkdir(parents=True, exist_ok=True)
    config = {"agent_name": "test-host", "portal_url": ORIGIN, "access_token": TOKEN,
              "token": secrets.token_hex(24), "server_addr": "127.0.0.1",
              "server_port": 18885, "remote_port": 18886, "local_addr": "127.0.0.1:18880",
              "apps": []}
    CONFIG.write_text(json.dumps(config))
    CONFIG.chmod(0o600)
    fixture = ThreadingHTTPServer(("127.0.0.1", 18881), Fixture)
    threading.Thread(target=fixture.serve_forever, daemon=True).start()
    proc = start_outpost()
    check(GRANTED.wait(15), "real outpost fetched the fake host grant copy")
    src = str(HOME / "website")
    run(*WEBSITE, "enable", src, "--site", ORIGIN, "--title", "Original support greeting")
    run(*WEBSITE, "register", src, "--name", "site")
    run(*WEBSITE, "serve", src, "--name", APP, "--port", str(PAGES))
    # website serve registers its page upstream. The CLI has no mcp-port
    # flag. Configure the app's documented mcp_port and reload the real daemon.
    proc.terminate()
    proc.wait(timeout=30)
    config = json.loads(CONFIG.read_text())
    app, = [app for app in config["apps"] if app["name"] == APP]
    check(app["trust_cloud_identity"] and app["sso_secret"], "website provisioned its signing key")
    app.update(mcp_port=ENGINE, require_login=True)
    CONFIG.write_text(json.dumps(config))
    GRANTED.clear()
    start_outpost()
    check(GRANTED.wait(15), "grants fetched after mounting MCP")
    engine = subprocess.Popen(["bashy", "mcp", "serve", "--transport", "http", "--tools", "site",
                               "--outpost", "--site-app", APP, "--listen", f"127.0.0.1:{ENGINE}"])
    ready(engine, ENGINE)
    scenario()
    print("PASS group shaping phase 1", flush=True)


if __name__ == "__main__":
    main()
