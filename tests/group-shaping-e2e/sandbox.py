"""Process supervisor and synthetic clients. Run only inside the disposable image."""
import base64
import difflib
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import Request, urlopen

APP = "site-shaping"
PAGE = "page"
PAGE_HTTP, PAGE_MCP = 18887, 18888
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
                    "groups": {"support": [A, C]}, "apps": {app: [{"groups": ["support"]}] for app in (APP, PAGE)}}
            GRANTED.set()
            mime, body = "application/json", json.dumps(data)
        elif self.path == "/index.json":
            mime, body = "application/json", json.dumps(
                {"path": "/", "title": "Support desk", "revision": "fixture-v1", "sections": []})
        elif self.path == "/index.md":
            mime, body = "text/markdown", "# Support desk\n"
        elif self.path in ("/", "/published/", "/member/"):
            greeting = {"/": "Original support greeting", "/published/": "Approved support greeting",
                        "/member/": "Member support greeting"}[self.path]
            mime, body = "text/html", f"<html><body><h1>{greeting}</h1></body></html>"
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


def headers(user, mount):
    result = {"X-Forwarded-Prefix": mount}
    if user:
        result["X-Periscope-User"] = user
    return result


def call(user, tool, args, app=APP):
    mount = "/app/" + app
    body = {"jsonrpc": "2.0", "id": 1, "method": "tools/call",
            "params": {"name": tool, "arguments": args}}
    hdr = dict(headers(user, mount), Accept="application/json, text/event-stream",
               **{"Content-Type": "application/json", "MCP-Protocol-Version": "2025-06-18"})
    req = Request(FRONT + mount + "/mcp", json.dumps(body).encode(), hdr)
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
                mount + '/.well-known/oauth-protected-resource"'):
            return {"challenge": True}
        raise


def success(user, tool, args, app=APP):
    result = call(user, tool, args, app)
    if result.get("isError") or "structuredContent" not in result:
        raise AssertionError(f"{tool}: {result}")
    return result["structuredContent"]


def shape(user, verb, args, app=APP):
    result = success(user, "shape_" + verb, args, app)
    check((result["principal"], result["app"], result["verb"]) == (user, app, verb),
          f"{verb} attributed to {user} on the mounted app")
    return result["result"]


def member_refused(member, tool, result):
    reason = f"refused: caller {member} (tier member) is not authorized for {tool}"
    check(result.get("isError") and reason in result["content"][0]["text"],
          f"{member} refused {tool} as a member")


def owner_cli(*args):
    cmd = ["bashy", "shape", *args, "--json"]
    print("+", *cmd, flush=True)
    raw = subprocess.check_output(cmd, env=dict(os.environ, BASHY_PRINCIPAL=OWNER),
                                  text=True, timeout=30)
    print(raw, flush=True)
    return json.loads(raw)


def get_page(user, path, app=APP):
    mount = "/app/" + app
    with urlopen(Request(FRONT + mount + path, headers=headers(user, mount)), timeout=120) as response:
        return response.read().decode()


def wait_version(revision):
    deadline = time.monotonic() + 60
    last = None
    while time.monotonic() < deadline:
        try:
            last = json.loads(get_page(C, "/_shaping/api/version"))["version"]
            if last == revision:
                return
        except HTTPError as exc:
            if exc.code not in (502, 503, 504):
                raise
            last = str(exc)
        except (URLError, ConnectionError) as exc:
            last = str(exc)
        time.sleep(.5)
    raise AssertionError(f"served version {last!r}, expected {revision}")


PAGE_BASE = "Open the link the organizer shared, or type the page here."
PAGE_EDIT = "Open the shared support page here."


def member_pull_push(active, app=APP):
    snapshot = shape(C, "pull", {}, app)
    base = snapshot["revision"]
    filename = "main.go" if app == PAGE else "site.json"
    source, = [f for f in snapshot["files"] if f["path"] == filename]
    with tempfile.TemporaryDirectory() as checkout:
        path = Path(checkout) / filename
        path.write_bytes(base64.b64decode(source["content"], validate=True))
        original = path.read_text()
        if app == PAGE:
            check(base == active and original.count(PAGE_BASE) == 1, "C pulled O's active Page source")
            path.write_text(original.replace(PAGE_BASE, PAGE_EDIT))
        else:
            config = json.loads(original)
            check(base == active and config["title"] == "Approved support greeting"
                  and config["site_url"] == ORIGIN + "/published", "C pulled O's active website source")
            config.update(title="Member support greeting", site_url=ORIGIN + "/member")
            path.write_text(json.dumps(config, indent=2) + "\n")
        patch = "".join(difflib.unified_diff(original.splitlines(True), path.read_text().splitlines(True),
                                             fromfile="a/" + filename, tofile="b/" + filename))
    push = {"base_revision": base, "patch": patch}
    for tool, args in (("shape_pull", {}), ("shape_push", push)):
        member_refused(A, tool, call(A, tool, args, app))
        for user in (ACCOUNT, None):
            check(call(user, tool, args, app).get("challenge"),
                  f"{user or 'anonymous'} refused {tool} by the mounted app grant gate")

    try:
        candidate = shape(C, "push", push, app)
    except AssertionError:
        for item in owner_cli("status", app)["candidates"]:
            evidence = item.get("verdict", {}).get("evidence")
            if evidence:
                print("push verifier evidence", Path(evidence).read_text(), flush=True)
        raise
    cid, rev = candidate["id"], candidate["rev"]
    check(cid and rev and rev != base and candidate["base_rev"] == base and
          candidate["principal"] == C and candidate["activation_principal"] == OWNER and
          candidate["status"] == "verified",
          "C's pushed patch produced a verified candidate on O's line")
    status = owner_cli("status", app)
    queued, = [item for item in status["candidates"] if item["id"] == cid]
    evidence = json.loads(Path(queued["verdict"]["evidence"]).read_text())
    print("push verifier evidence", json.dumps(evidence), flush=True)
    binding = evidence["binding"]
    verifier = "checks/go.bsh" if app == PAGE else "checks/build.bsh"
    check(queued["verdict"]["pass"] and
          (binding["principal"], binding["base"], binding["candidate"]) == (OWNER, base, rev)
          and evidence["verdict"] == "pass" and
          any(c["name"] == verifier and c["status"] == "pass" for c in evidence["checks"]),
          f"O's {app} verifier passed the pushed revision against the pulled base")
    if app == PAGE:
        html = get_page(C, "/", PAGE)
        served_base = PAGE_BASE in html and PAGE_EDIT not in html and "data-bashy-genie" in html
    else:
        served_base = json.loads(get_page(C, "/_shaping/api/version"))["version"] == base
    check(status["active"] == base and served_base,
          "O's active and served versions stay at the pulled base pending approval")

    approved = shape(OWNER, "approve", {"id": cid}, app)
    check(approved["id"] == cid and approved["rev"] == rev and approved["status"] == "approved",
          "O approved C's pushed revision")
    history = owner_cli("history", app)
    entry, = [v for v in history["versions"] if v["revision"] == rev]
    check(history["active"] == rev and entry["principal"] == OWNER and entry["author"] == C,
          "O's history credits C for the pushed revision")
    if app == PAGE:
        html = get_page(C, "/", PAGE)
        check(PAGE_EDIT in html and PAGE_BASE not in html and "data-bashy-genie" in html,
              "the mounted Page serves C's approved text")
    else:
        wait_version(rev)
        panel = json.loads(get_page(C, "/_shaping/api/me"))
        check(panel["version"] == rev and panel["title"] == "Member support greeting"
              and "<h1>Member support greeting</h1>" in get_page(C, "/"),
              "the mounted site serves C's local edit at the approved revision")

    stale = call(C, "shape_push", push, app)
    check(stale.get("isError") and f"active revision changed (base {base}, active {rev})"
          in stale["content"][0]["text"], "stale push refused with both pulled and active revisions")
    check(owner_cli("status", app)["active"] == rev, "stale push left the approved revision active")


def queued_wish(app):
    req = shape(A, "wish", {"request": "Change the support page footer", "agent": BINDING}, app)
    status = owner_cli("status", app)
    queued, = [item for item in status["candidates"] if item["id"] == req["id"]]
    check(req == queued and queued["kind"] == "request" and queued["status"] == "requested"
          and queued["principal"] == A and queued["activation_principal"] == OWNER,
          "A's code wish is a request in O's queue")
    return queued


def scenario_request_workspace(candidate, queued):
    request_dir = Path(candidate["workspace"]).parents[1] / queued["id"]
    check(not queued.get("workspace") and (request_dir / "candidate.json").is_file()
          and not (request_dir / "work").exists(), "A's request has no workspace")


def set_scopes(app):
    for scope, circle in (("edit", "company"), ("shape", "contractor")):
        policy = owner_cli("scope", "set", "--app", app, scope, "--circle", circle)
        check(policy["scopes"][scope]["circles"] == [circle], f"owner set {scope} for {circle}")


def scenario():
    wish = {"request": "Publish the support greeting", "agent": BINDING}
    set_scopes(APP)
    panel = json.loads(get_page(OWNER, "/_shaping/api/me"))
    before = json.loads(get_page(C, "/_shaping/api/version"))["version"]
    check(panel["tier"] == "owner" and panel["title"] == "Original support greeting"
          and before and before == panel["version"], "panel shows the owner's initial served version")
    check("<h1>Original support greeting</h1>" in get_page(C, "/"), "mounted page shows the base greeting")
    for member, scope in ((A, "edit"), (C, "shape")):
        panel = json.loads(get_page(member, "/_shaping/api/me"))
        check(panel["tier"] == "member" and panel["scope"] == scope, f"panel grants {member} {scope}")
        page = success(member, "read_page", {"path_or_url": "/", "format": "md"})
        check(page["content"].strip() == "# Support desk" and page["revision"] == "fixture-v1",
              f"{member} read the served page through MCP")
    for user in (NON_SUPPORT, ACCOUNT, None):
        check(call(user, "shape_wish", wish).get("challenge"),
              f"{user or 'anonymous'} refused by the mounted app grant gate")
    candidate = shape(C, "wish", wish)
    cid, rev = candidate["id"], candidate["rev"]
    base = candidate["base_rev"]
    check(candidate["principal"] == C and candidate["activation_principal"] == OWNER,
          "C created a candidate for the owner's line")
    check(cid and rev and candidate["base_rev"] != rev and candidate["status"] == "ready",
          "wish produced a new ready revision")
    check((HOME / "stub-events").read_text().splitlines() == ["edit"],
          "exactly one fixed edit ran through the delegate")
    diff = subprocess.check_output(["bashy", "shape", "diff", APP, cid, "--user", OWNER], text=True)
    print(diff, flush=True)
    check('-  "title": "Original support greeting"' in diff and
          '+  "title": "Approved support greeting"' in diff, "candidate contains the fixed edit")
    verified = shape(C, "verify", {"id": cid})
    check(verified["id"] == cid and verified["rev"] == rev and verified["status"] == "verified"
          and verified["verdict"]["pass"], "trusted verifier passed the candidate revision")
    evidence = json.loads(Path(verified["verdict"]["evidence"]).read_text())
    print("verifier evidence", json.dumps(evidence), flush=True)
    check(evidence["binding"]["candidate"] == rev and evidence["binding"]["principal"] == OWNER and
          any(c["name"] == "checks/build.bsh" and c["status"] == "pass" for c in evidence["checks"]),
          "sealed verifier evidence records the website check on the owner's revision")
    for tool in ("shape_verify", "shape_preview", "shape_approve"):
        member_refused(A, tool, call(A, tool, {"id": cid}))
    member_refused(C, "shape_approve", call(C, "shape_approve", {"id": cid}))
    preview = shape(C, "preview", {"id": cid})
    check(preview["candidate"]["rev"] == rev and
          urlsplit(preview["url"]).path == f"/shape/{APP}/preview/{cid}/",
          "MCP returned the isolated preview for C's verified candidate")
    check(owner_cli("status", APP)["active"] == base and
          json.loads(get_page(C, "/_shaping/api/version"))["version"] == before,
          "preview left the owner's active and served versions unchanged")

    other = shape(OWNER, "wish", {"request": "Draft the alternate greeting", "agent": BINDING})
    check(other["base_rev"] == base and other["id"] != cid and other["rev"] not in (base, rev)
          and other["status"] == "ready", "second candidate contains a different edit on the same base")
    approved = shape(OWNER, "approve", {"id": cid})
    check(approved["id"] == cid and approved["rev"] == rev and approved["status"] == "approved",
          "owner approved C's verified revision")
    history = owner_cli("history", APP)
    entry, = history["versions"]
    check(history["active"] == rev and entry["revision"] == rev and
          entry["principal"] == OWNER and entry["author"] == C, "owner's history credits C")
    wait_version(rev)
    panel = json.loads(get_page(C, "/_shaping/api/me"))
    check(rev != before and panel["version"] == rev and panel["title"] == "Approved support greeting",
          "member panel follows the approved version")
    check("<h1>Approved support greeting</h1>" in get_page(C, "/"),
          "mounted page serves the approved greeting")

    scenario_request_workspace(candidate, queued_wish(APP))
    check((HOME / "stub-events").read_text().splitlines() == ["edit", "alternate"],
          "only the two candidate wishes invoked the stub")

    result = call(OWNER, "shape_approve", {"id": other["id"]})
    check(result.get("isError") and f"active revision changed (base {base}, active {rev})"
          in result["content"][0]["text"], "stale approval refused with both base and active revisions")
    check(owner_cli("status", APP)["active"] == rev, "stale approval left C's revision active")
    member_pull_push(rev)


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
    for adapter in ("delegate", "mod"):
        shutil.copy("/tools/" + adapter, cache / adapter)
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
    config = {"agent_name": "test-host", "oauth2_email": OWNER, "portal_url": ORIGIN, "access_token": TOKEN,
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
    run(*WEBSITE, "serve", src, "--name", APP, "--port", str(PAGES), "--app", APP, "--principal", OWNER)
    os.environ.update(GOPROXY="file:///page-deps,off", CGO_ENABLED="0")
    # Match the official build image's binmgr layout using the read-only SDK.
    version = subprocess.check_output(["/go/bin/go", "env", "GOVERSION"], text=True).strip()
    tool_cache = Path(os.environ["BASHY_BIN_CACHE"]) / "go" / version.removeprefix("go")
    tool_cache.mkdir(parents=True)
    (tool_cache / "go").symlink_to("/go", target_is_directory=True)
    (tool_cache / ".binmgr-tree-complete").write_text("ok\n")
    run("bash", "/desk/page/register.sh")
    run("outpost", "apps", "add", PAGE, "--url", f"http://127.0.0.1:{PAGE_HTTP}/page/",
        "--trust-cloud-identity", "--require-login")
    # website serve registers its page upstream. The CLI has no mcp-port
    # flag. Configure the app's documented mcp_port and reload the real daemon.
    proc.terminate()
    proc.wait(timeout=30)
    config = json.loads(CONFIG.read_text())
    app, = [app for app in config["apps"] if app["name"] == APP]
    check(app["trust_cloud_identity"] and app["sso_secret"], "website provisioned its signing key")
    app.update(mcp_port=ENGINE, require_login=True)
    page, = [app for app in config["apps"] if app["name"] == PAGE]
    check(page["trust_cloud_identity"] and page["sso_secret"], "Page mount has its own signing key")
    page["mcp_port"] = PAGE_MCP
    CONFIG.write_text(json.dumps(config))
    GRANTED.clear()
    start_outpost()
    check(GRANTED.wait(15), "grants fetched after mounting MCP")
    for name, port in ((APP, ENGINE), (PAGE, PAGE_MCP)):
        engine = subprocess.Popen(["bashy", "mcp", "serve", "--transport", "http", "--tools", "site",
                                   "--outpost", "--site-app", name, "--listen", f"127.0.0.1:{port}"])
        ready(engine, port)
    scenario()
    console = subprocess.Popen(["bashy", "app", "serve", "--port", str(PAGE_HTTP)])
    ready(console, PAGE_HTTP)
    set_scopes(PAGE)
    queued_wish(PAGE)
    member_pull_push(owner_cli("status", PAGE)["active"], PAGE)
    print("PASS scoped group shaping", flush=True)


if __name__ == "__main__":
    main()
