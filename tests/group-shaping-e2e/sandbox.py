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
REPAIR_AGENT = "codex:gpt6-terra"
X_WHY = "visitors must see the approved support greeting"
X_EXPECT = "the served heading reads Approved support greeting"
C_WHY = "the support page should greet members by their own team"
C_EXPECT = "members see Member support greeting on the support page"


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


def shape(user, verb, args):
    result = success(user, "shape_" + verb, args)
    check((result["principal"], result["app"], result["verb"]) == (user, APP, verb),
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


def owner_candidate(cid):
    """The owner's full record of a candidate; a member's MCP reply is filtered."""
    found, = [item for item in owner_cli("status", APP)["candidates"] if item["id"] == cid]
    return found


HOST_FIELDS = ("workspace", "tracking_root", "tracking_item", "artifact_path", "actor",
               "activation_principal", "agent_identity", "agent_receipt")


def member_view(reply, full):
    """A member's reply is the owner's record without host paths and identities."""
    verdict = reply.get("verdict") or {}
    return (not any(field in reply for field in HOST_FIELDS)
            and "artifact_path" not in verdict
            and verdict.get("evidence", "shape-evidence:" + reply.get("rev", "")) == "shape-evidence:" + reply.get("rev", "")
            and all(reply[k] == full[k] for k in ("id", "principal", "status", "request")))


def get_page(user, path):
    with urlopen(Request(FRONT + MOUNT + path, headers=headers(user)), timeout=5) as response:
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


def ledger_problem(ledger, cid):
    """The problem holding candidate cid in a ledger view."""
    for problem in ledger["problems"]:
        if any(a.get("candidate") == cid for a in problem.get("attempts", [])):
            return problem
    raise AssertionError(f"no problem holds candidate {cid}: {json.dumps(ledger)}")


def standings(problem):
    return [(a.get("agent"), a["standing"]) for a in problem["attempts"]]


def last_prompt():
    """The stub's last recorded prompt and the work ledger it found, if any."""
    block = (HOME / "stub-prompts").read_text().split("=== prompt\n")[-1]
    prompt, _, ledger = block.partition("=== work ledger\n")
    return prompt, ledger


def ledger_leg(cid, rev):
    """X's wish is in the ledger; Y repairs an incident with X's reasons in its brief; undo and restore change standing only."""
    ledger = owner_cli("ledger", APP, "--max-bytes", "-1")
    problem = ledger_problem(ledger, cid)
    wish, = [a for a in problem["attempts"] if a.get("candidate") == cid]
    check(wish["agent"] == BINDING and wish["principal"] == C and wish["why"] == X_WHY
          and wish["expect"] == X_EXPECT and wish["standing"] == "active",
          "the ledger records X's wish for C with its why, expect and active standing")

    # Unattended repair ships as a patch release of a named base release.
    owner_cli("release", APP, rev)
    incident = owner_cli("incident", APP, "Script error on the support page", "--kind", "script",
                         "--path", "/", "--agent", REPAIR_AGENT)
    repaired = owner_cli("status", APP)["active"]
    check(repaired not in (rev, "") and (HOME / "stub-events").read_text().splitlines()[-1] == "repair",
          f"Y repaired incident {incident.get('id')} and the repair activated by itself")
    prompt, work_ledger = last_prompt()
    fenced = prompt.split("<<<LEDGER\n", 1)[-1].split("\nLEDGER>>>", 1)[0]
    print("repair prompt", prompt, "work ledger", work_ledger, sep="\n", flush=True)
    check("<<<INCIDENT" in prompt and "<<<LEDGER\n" in prompt and X_WHY in fenced and X_EXPECT in fenced,
          "Y's repair brief carries X's why and expect inside the ledger fence")
    check(X_WHY in work_ledger and json.loads(work_ledger)["problems"],
          "a step in Y's work clone read .git/shape-ledger.json with X's reasons")

    after = ledger_problem(owner_cli("ledger", APP, "--max-bytes", "-1"), cid)
    check(after["key"] == problem["key"] and incident.get("id") in after.get("incidents", []),
          "the incident is filed under X's problem")
    check(standings(after) == [(BINDING, "superseded"), (REPAIR_AGENT, "active")],
          "Y's repair is active and X's wish superseded")
    count = len(after["attempts"])

    owner_cli("undo", APP)
    check(owner_cli("status", APP)["active"] == rev, "undo made X's revision active again")
    undone = ledger_problem(owner_cli("ledger", APP, "--max-bytes", "-1"), cid)
    check(standings(undone) == [(BINDING, "active"), (REPAIR_AGENT, "undone")] and len(undone["attempts"]) == count,
          "undo marked Y's repair undone and deleted nothing")
    owner_cli("restore", APP, repaired)
    check(owner_cli("status", APP)["active"] == repaired, "restore made Y's repair active again")
    restored = ledger_problem(owner_cli("ledger", APP, "--max-bytes", "-1"), cid)
    check(standings(restored) == [(BINDING, "superseded"), (REPAIR_AGENT, "restored")]
          and len(restored["attempts"]) == count, "restore marked Y's repair restored and deleted nothing")
    wait_version(repaired)
    return repaired


def member_pull_push(active, title):
    snapshot = shape(C, "pull", {})
    base = snapshot["revision"]
    seen = json.dumps(snapshot.get("ledger"))
    check(snapshot.get("ledger") and X_WHY in seen and OWNER not in seen and str(HOME) not in seen
          and "/shape-work/" not in seen, "C's pull carries the ledger without the owner's address or host paths")
    site, = [f for f in snapshot["files"] if f["path"] == "site.json"]
    with tempfile.TemporaryDirectory() as checkout:
        path = Path(checkout) / "site.json"
        path.write_bytes(base64.b64decode(site["content"], validate=True))
        original = path.read_text()
        config = json.loads(original)
        check(base == active and config["title"] == title
              and config["site_url"] == ORIGIN + "/published", "C pulled O's active website source")
        config.update(title="Member support greeting", site_url=ORIGIN + "/member")
        path.write_text(json.dumps(config, indent=2) + "\n")
        patch = "".join(difflib.unified_diff(original.splitlines(True), path.read_text().splitlines(True),
                                             fromfile="a/site.json", tofile="b/site.json"))
    push = {"base_revision": base, "patch": patch}
    for tool, args in (("shape_pull", {}), ("shape_push", push)):
        member_refused(A, tool, call(A, tool, args))
        for user in (ACCOUNT, None):
            check(call(user, tool, args).get("challenge"),
                  f"{user or 'anonymous'} refused {tool} by the mounted app grant gate")

    candidate = shape(C, "push", dict(push, why=C_WHY, expect=C_EXPECT, agent="contractor-local-model"))
    cid, rev = candidate["id"], candidate["rev"]
    attempt, = [a for a in ledger_problem(owner_cli("ledger", APP, "--max-bytes", "-1"), cid)["attempts"]
                if a.get("candidate") == cid]
    check(attempt["principal"] == C and attempt["why"] == C_WHY and attempt["expect"] == C_EXPECT
          and attempt["agent"] == "contractor-local-model" and attempt["request"].startswith(C_WHY),
          "C's push is in the ledger with C as principal, C's reasons and C's agent")
    check(cid and rev and rev != base and candidate["base_rev"] == base and
          candidate["principal"] == C and "activation_principal" not in candidate and
          candidate["status"] == "verified",
          "C's pushed patch produced a verified candidate; the reply carries no owner identity")
    status = owner_cli("status", APP)
    queued, = [item for item in status["candidates"] if item["id"] == cid]
    check(queued.get("activation_principal") == OWNER, "C's pushed candidate activates on O's line")
    evidence = json.loads(Path(queued["verdict"]["evidence"]).read_text())
    print("push verifier evidence", json.dumps(evidence), flush=True)
    binding = evidence["binding"]
    check(queued["verdict"]["pass"] and
          (binding["principal"], binding["base"], binding["candidate"]) == (OWNER, base, rev)
          and evidence["verdict"] == "pass" and
          any(c["name"] == "checks/build.bsh" and c["status"] == "pass" for c in evidence["checks"]),
          "O's website verifier passed the pushed revision against the pulled base")
    check(status["active"] == base and
          json.loads(get_page(C, "/_shaping/api/version"))["version"] == base,
          "O's active and served versions stay at the pulled base pending approval")

    approved = shape(OWNER, "approve", {"id": cid})
    check(approved["id"] == cid and approved["rev"] == rev and approved["status"] == "approved",
          "O approved C's pushed revision")
    history = owner_cli("history", APP)
    entry, = [v for v in history["versions"] if v["revision"] == rev]
    check(history["active"] == rev and entry["principal"] == OWNER and entry["author"] == C,
          "O's history credits C for the pushed revision")
    wait_version(rev)
    panel = json.loads(get_page(C, "/_shaping/api/me"))
    check(panel["version"] == rev and panel["title"] == "Member support greeting"
          and "<h1>Member support greeting</h1>" in get_page(C, "/"),
          "the mounted site serves C's local edit at the approved revision")

    stale = call(C, "shape_push", push)
    check(stale.get("isError") and f"active revision changed (base {base}, active {rev})"
          in stale["content"][0]["text"], "stale push refused with both pulled and active revisions")
    check(owner_cli("status", APP)["active"] == rev, "stale push left the approved revision active")


def scenario():
    wish = {"request": "Publish the support greeting", "agent": BINDING}
    for scope, circle in (("edit", "company"), ("shape", "contractor")):
        policy = owner_cli("scope", "set", "--app", APP, scope, "--circle", circle)
        check(policy["scopes"][scope]["circles"] == [circle], f"owner set {scope} for {circle}")
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
    owned = owner_candidate(cid)
    check(candidate["principal"] == C and owned["activation_principal"] == OWNER,
          "C created a candidate for the owner's line")
    check(member_view(candidate, owned) and owned["workspace"],
          "C's wish reply carries no host path or identity")
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
    check(verified["verdict"]["evidence"] == "shape-evidence:" + rev and member_view(verified, owner_candidate(cid)),
          "C's verify reply names the evidence by reference, not by host path")
    evidence = json.loads(Path(owner_candidate(cid)["verdict"]["evidence"]).read_text())
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

    req = shape(A, "wish", {"request": "Change the support page footer", "agent": BINDING})
    status = owner_cli("status", APP)
    queued, = [item for item in status["candidates"] if item["id"] == req["id"]]
    check(member_view(req, queued) and queued["kind"] == "request" and queued["status"] == "requested"
          and queued["principal"] == A and queued["activation_principal"] == OWNER,
          "A's code wish is a request in O's queue")
    request_dir = Path(owned["workspace"]).parents[1] / req["id"]
    check(not queued.get("workspace") and (request_dir / "candidate.json").is_file()
          and not (request_dir / "work").exists(), "A's request has no workspace")
    check((HOME / "stub-events").read_text().splitlines() == ["edit", "alternate"],
          "only the two candidate wishes invoked the stub")

    result = call(OWNER, "shape_approve", {"id": other["id"]})
    check(result.get("isError") and f"active revision changed (base {base}, active {rev})"
          in result["content"][0]["text"], "stale approval refused with both base and active revisions")
    check(owner_cli("status", APP)["active"] == rev, "stale approval left C's revision active")
    repaired = ledger_leg(cid, rev)
    member_pull_push(repaired, "Approved support greeting (repaired)")


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
    invocation = json.loads(subprocess.check_output(
        ["bashy", "delegate", "--dry-run", "--json", "--agent", REPAIR_AGENT, "binding preflight"],
        env=dict(os.environ, BASHY_DELEGATE_CALLER="genie", BASHY_DELEGATE_ALLOW=REPAIR_AGENT)))
    check(invocation["agent"] == stub, f"{REPAIR_AGENT} resolves to {stub}")
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
    print("PASS scoped group shaping", flush=True)


if __name__ == "__main__":
    main()
