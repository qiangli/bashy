"""TaaS A/O/C scenario: supervisor and synthetic clients. Run only inside the disposable image.

C is the container host (offerer): its door and the real outpost. A and O are
members who reach C's offer apps through the front (outpost signs the stamp).
A second BASHY_HOME door plays O's owned host for the peer path and for the
symmetry check. No LLM anywhere: a fake OpenAI-compatible server answers the
model, a stub answers the agent, and transcribe is a registered script.

T1 (usage rollups) and T2 (mounted tools) checks run only with TAAS_T1=1 and
TAAS_T2=1, so the gate can run before both stories land.
"""
import base64
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError
from urllib.request import Request, urlopen

sys.path.insert(0, "/group-shaping")
from sandbox import check, ready, run  # noqa: E402  the group-shaping helpers

FRONT = "http://127.0.0.1:18880"
ORIGIN = "http://127.0.0.1:18881"
ADMIN = "127.0.0.1:18882"
MODEL_PORT = 18890
C_PORT, O_PORT = 24556, 24600
OWNER, A, O, NON_MEMBER = [f"{n}@example.invalid" for n in ("c", "a", "o", "member")]
TOKEN = secrets.token_hex(24)
GRANTED = threading.Event()
HOME = Path.home()
C_HOME, O_HOME = HOME / ".bashy", HOME / "o-host"
CONFIG = HOME / ".config/matrix/agent.json"
MODEL, AGENT, CAP = "fake-model", "helper", 64
OFFERS = ("llm", "stt", AGENT, "files-docs", "files-outside")
T1, T2 = os.environ.get("TAAS_T1") == "1", os.environ.get("TAAS_T2") == "1"
WAIT = float(os.environ.get("TAAS_WAIT", "15"))
calls = {}  # (principal, tool) -> list of usage dicts the callers saw


class Satellite(BaseHTTPRequestHandler):
    """The fake satellite's grant copy (cloudboxfake accounts): A and O are in support."""

    def do_GET(self):
        if self.path != "/api/v1/hosts/self/grants" or self.headers.get("Authorization") != "Bearer " + TOKEN:
            self.send_error(401 if self.path.endswith("/grants") else 404)
            return
        data = {"host": "c-host", "owner": OWNER, "revision": 1,
                "members": [{"email": A, "circle": "company"}, {"email": O, "circle": "company"},
                            {"email": NON_MEMBER, "circle": "company"}],
                "groups": {"support": [A, O]},
                "apps": {"offer-" + name: [{"groups": ["support"]}] for name in OFFERS}}
        GRANTED.set()
        raw = json.dumps(data).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, *_):
        pass


class FakeModel(BaseHTTPRequestHandler):
    """A tiny OpenAI-compatible server: a fixed answer with fixed usage."""

    def do_GET(self):
        self.reply({"object": "list", "data": [{"id": MODEL, "object": "model"}]})

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
        print("fake model", self.path, json.dumps(body)[:200], flush=True)
        self.reply({"id": "cmpl-1", "object": "chat.completion", "model": body.get("model", MODEL),
                    "choices": [{"index": 0, "finish_reason": "stop",
                                 "message": {"role": "assistant", "content": "fake answer"}}],
                    "usage": {"prompt_tokens": 7, "completion_tokens": 3, "total_tokens": 10}})

    def reply(self, data):
        raw = json.dumps(data).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, *_):
        pass


TRANSCRIBE = f"""name: stt
kind: command
synopsis: transcribe an audio file (fake provider)
script: |
  in=""; while [ $# -gt 0 ]; do case $1 in --input) in=$2; shift 2;; *) shift;; esac; done
  n=$(wc -c < "$in")
  printf 'transcript of %s bytes\\n' "$n" > out.txt
  printf '{{"text":"transcript of %s bytes","seconds":1}}\\n' "$n"
effects: [read]
unit: seconds
args:
  flags:
    - name: input
      type: string
      required: true
"""
UNTYPED = """name: untyped
kind: command
synopsis: a command with no args schema
script: echo untyped
effects: [read]
"""


def as_host(home, port):
    return dict(os.environ, BASHY_HOME=str(home), BASHY_LLM_PORT=str(port))


def cli(env, *args, ok=True):
    """Run bashy and print the command and its output as evidence."""
    print("+ bashy", *args, flush=True)
    proc = subprocess.run(["bashy", *args], env=env, capture_output=True, text=True, timeout=120)
    print(proc.stdout + proc.stderr, flush=True)
    if ok and proc.returncode:
        raise AssertionError(f"bashy {' '.join(args)} exited {proc.returncode}")
    return proc


def cli_json(env, *args):
    return json.loads(cli(env, *args).stdout)


def http(method, url, body=None, hdr=None):
    """(status, headers, bytes) for one request; errors return their status."""
    data = json.dumps(body).encode() if body is not None else None
    req = Request(url, data, dict({"Content-Type": "application/json"}, **(hdr or {})), method=method)
    try:
        with urlopen(req, timeout=120) as res:
            status, headers, raw = res.status, res.headers, res.read()
    except HTTPError as exc:
        status, headers, raw = exc.code, exc.headers, exc.read()
    print(method, url, status, raw[:400].decode(errors="replace"), flush=True)
    return status, headers, raw


def member(user, app, path, method="POST", body=None, hdr=None):
    """A member's request to C's offer app through the front."""
    h = {"X-Forwarded-Prefix": "/app/offer-" + app}
    if user:
        h["X-Periscope-User"] = user
    return http(method, FRONT + "/app/offer-" + app + path, body, dict(h, **(hdr or {})))


def member_call(user, name, args, expect=200):
    status, _, raw = member(user, name, "/offers/" + name, body={"args": args})
    check(status == expect, f"{user or 'anonymous'} calls {name} through the front: HTTP {status}")
    if status == 200:
        reply = json.loads(raw)
        calls.setdefault((user, name), []).append(reply.get("usage"))
        return reply
    return raw.decode(errors="replace")


def prompt_args(definition, target):
    """Fill the definition's required string arguments with a short prompt."""
    props = definition["input"].get("properties", {})
    return {k: "say hello" for k in definition["input"].get("required", [])
            if k != target and props.get(k, {}).get("type", "string") == "string"}


def start_outpost():
    proc = subprocess.Popen(["outpost", "start"])
    ready(proc, 18880)
    return proc


def door(env, port):
    proc = subprocess.Popen(["bashy", "llm", "serve", "--no-engine", "--port", str(port)], env=env)
    ready(proc, port)
    return proc


def peer(owner_env, caller_env, owner_name, caller_name, owner_port, commands):
    """Mint a scoped peer credential on the owner door and save it in the caller's peers file."""
    cred = cli(owner_env, "llm", "peer", "add", caller_name, "--share", "allow").stdout.strip().splitlines()[-1]
    cli(owner_env, "llm", "peer", "set", caller_name, *sum((["--command", c] for c in commands), []))
    path = Path(caller_env["BASHY_HOME"]) / "toolcmd/peers.json"
    path.parent.mkdir(parents=True, exist_ok=True)
    peers = json.loads(path.read_text()) if path.exists() else {"peers": {}}
    peers["peers"][owner_name] = {"url": f"http://127.0.0.1:{owner_port}", "credential": cred}
    path.write_text(json.dumps(peers))
    path.chmod(0o600)
    return cred


def restart_outpost(proc):
    """Reload the real daemon so the offer apps added through its CLI are served."""
    proc.terminate()
    proc.wait(timeout=30)
    GRANTED.clear()
    proc = start_outpost()
    check(GRANTED.wait(WAIT), "grants fetched after the offer apps were added")
    return proc


def scenario(c, o, outpost):
    # (1) local calls on C.
    reply = cli_json(c, "llm", "call", "--model", MODEL, "--prompt", "say hello", "--json")
    check("fake answer" in json.dumps(reply), "bashy llm call answers locally on C from the fake model")
    agent = cli(c, "delegate", "--json", "--agent", AGENT, "--max-tokens", str(CAP), "say hello", ok=False)
    check(agent.returncode == 0 or "token" in agent.stderr.lower(),
          f"the agent tool runs locally on C or names its token cap (exit {agent.returncode})")

    # (2) offers: one of each class; an internal verb and an untyped command are refused.
    for name, extra in (("llm", ["--limit", "0"]), ("stt", ["--limit", "1"]),
                        (AGENT, ["--token-cap", str(CAP)])):
        target = MODEL if name == "llm" else name
        out = cli_json(c, "offer", target, "--group", "support", *extra, "--json")
        check(out["consumer"] == "bashy remote call HOST " + target and out["class"] in ("model", "tool", "agent"),
              f"C offered {target} ({out['class']}, {out['unit']}): {out['consumer']}")
    for refused in ("cat", "untyped"):
        proc = cli(c, "offer", refused, "--group", "support", ok=False)
        check(proc.returncode and "cannot offer" in proc.stderr, f"offering {refused} is refused")

    # Folders: C's managed docs folder, an outside --path, HOME without --force.
    docs = C_HOME / "shared/docs"
    docs.mkdir(parents=True, exist_ok=True)
    (docs / "readme.txt").write_text("docs shared in place\n")
    (docs / "out").symlink_to("/etc")
    outside = HOME / "outside"
    outside.mkdir()
    (outside / "note.txt").write_text("outside path\n")
    (HOME / "secret.txt").write_text("not shared\n")
    cli(c, "files", "share", "docs", "--group", "support")
    cli(c, "files", "share", "outside", "--path", str(outside), "--group", "support")
    proc = cli(c, "files", "share", "home", "--path", str(HOME), ok=False)
    check(proc.returncode and "--force" in proc.stderr, "sharing HOME without --force is refused")

    outpost = restart_outpost(outpost)

    # (8) symmetry: O offers its in folder to C as a peer; C reads it.
    (O_HOME / "shared/in").mkdir(parents=True, exist_ok=True)
    (O_HOME / "shared/in/in.wav").write_bytes(b"RIFF" + bytes(60))
    cli(o, "files", "share", "in")
    peer(o, c, "o", "c", O_PORT, ["files-in"])
    got = cli(c, "files", "cat", "bashy://o/files-in/in.wav", ok=False)
    check(got.returncode == 0 and got.stdout.startswith("RIFF"), "O offered files-in to C and C read in.wav")
    peer(c, o, "c", "o", C_PORT, ["llm", MODEL, "stt", "files-offers", "files-docs"])

    # (3) member calls through the front.
    status, _, raw = member(A, MODEL, f"/offers/{MODEL}?definition", "GET")
    definition = json.loads(raw)["definition"]
    check(status == 200 and definition["class"] == "model", "A read the llm offer's typed definition")
    reply = member_call(A, MODEL, prompt_args(definition, "model"))
    check("fake answer" in json.dumps(reply), "A's wish ran on C's LLM through the offer")
    reply = member_call(O, MODEL, prompt_args(definition, "model"))
    check("fake answer" in json.dumps(reply), "O's call ran on C's LLM through the offer")
    reply = member_call(A, "stt", {"input": "bashy://o/files-in/in.wav"})
    refs = reply["result"].get("artifact_refs") or reply["result"].get("ArtifactRefs") or []
    check(refs and "/files-offers/" in refs[0]["uri"], f"transcribe returned an artifact URI {refs and refs[0]['uri']}")
    member_call(A, "stt", {"input": "bashy://o/files-in/in.wav"}, expect=429)
    remote = cli(o, "remote", "call", "c", "stt", "--input", "bashy://o/files-in/in.wav", "--json")
    peer_reply = json.loads(remote.stdout)
    calls.setdefault(("peer:o", "stt"), []).append(peer_reply.get("usage"))
    uri = (peer_reply["result"].get("artifact_refs") or [{}])[0].get("uri", "")
    run_path = uri.split("/files-offers/", 1)[-1]
    status, headers, raw = http("GET", f"http://127.0.0.1:{C_PORT}/files/offers/{run_path}",
                                hdr={"Authorization": "Bearer " + json.loads(
                                    (O_HOME / "toolcmd/peers.json").read_text())["peers"]["c"]["credential"],
                                     "Range": "bytes=0-9"})
    check(status == 206 and raw == b"transcript" and "bytes 0-9/" in headers.get("Content-Range", ""),
          f"O ran bashy remote call c transcribe and streamed {uri} with a Range request")

    # A's agent reads C's folder in place; A reads the outside path.
    for name, path, want in (("docs", "readme.txt", "docs shared in place"), ("outside", "note.txt", "outside path")):
        reply = member_call(A, "files-" + name, {"op": "read", "path": path})
        check(want in base64.b64decode(reply["result"].get("data", "")).decode(),
              f"A read {path} from C's {name} share through the typed files tool")

    # (4) the agent offer stops at its cap.
    status, _, raw = member(O, AGENT, f"/offers/{AGENT}", body={"args": {"instruction": "say hello", "token_cap": CAP + 1}})
    check(status == 400 and b"exceeds offer cap" in raw, "an agent call above the offer's token cap is refused")
    status, _, raw = member(O, AGENT, f"/offers/{AGENT}", body={"args": {"instruction": "say hello"}})
    if status == 200:
        usage = json.loads(raw).get("usage") or {}
        calls.setdefault((O, AGENT), []).append(usage)
        total = int(usage.get("input", 0)) + int(usage.get("output", 0))
        check(total <= CAP, f"the agent call stayed within its cap: {total} <= {CAP} tokens")
    else:
        check(status == 400, f"the agent adapter refused a cap it cannot enforce: {raw[:200]!r} (recorded)")

    # (7) refusals.
    for user in (NON_MEMBER, None):
        status, _, _ = member(user, MODEL, f"/offers/{MODEL}", body={"args": {}})
        check(status in (401, 403), f"{user or 'anonymous'} is refused the llm offer: HTTP {status}")
    for bad in ("../secret.txt", "out/hostname"):
        status, _, _ = member(A, "files-docs", "/offers/files-docs", body={"args": {"op": "read", "path": bad}})
        check(400 <= status < 500, f"path escape {bad} is refused: HTTP {status}")
    status, _, _ = member(A, MODEL, "/offers/stt", body={"args": {}})
    check(status == 403, f"a command not offered on this app is refused: HTTP {status}")
    proc = cli(o, "remote", "call", "c", "untyped", ok=False)
    check(proc.returncode and "403" in proc.stderr + proc.stdout, "a peer calling a non-offered command is refused 403")

    # (6) T2: mounted tools.
    if T2:
        for name, args in (("stt", ["--input", "bashy://o/files-in/in.wav"]),
                           ("files-docs", ["--op", "ls"])):
            cli(o, "remote", "add", "c", name)
            tools = cli_json(o, "mcp", "tools", "--json")
            mounted = [t for t in tools if t.get("name", "").endswith(name)]
            check(mounted and mounted[0].get("inputSchema", {}).get("properties"),
                  f"bashy remote add c {name} shows a typed tool in bashy mcp tools")
            out = cli(o, "mcp", "call", mounted[0]["name"], *args, ok=False)
            check(out.returncode == 0, f"a call through the mounted {name} works")
        cli(c, "offer", "stop", MODEL)
        cli(c, "offer", MODEL, "--group", "support", "--tag", "changed")
        cli(o, "remote", "add", "c", MODEL)
        cli(c, "offer", "stop", MODEL)
        cli(c, "offer", MODEL, "--group", "support", "--tag", "changed-again")
        out = cli(o, "mcp", "call", "c-" + MODEL, "--prompt", "say hello", ok=False)
        check(out.returncode and "409" in out.stdout + out.stderr, "a mounted tool whose definition digest changed refuses 409")

    # (5) T1: both usage rollups agree per tool, in the offer's unit, with no price.
    if T1:
        offered = cli_json(c, "offer", "usage", "--json")
        remote = cli_json(o, "remote", "usage", "--json")
        text = json.dumps(offered) + json.dumps(remote)
        check("price" not in text and "usd" not in text.lower(), "no usage record carries a price")
        for tool, unit in ((MODEL, "token"), (AGENT, "token"), ("stt", "seconds"), ("files-docs", "bytes")):
            mine = [r for r in offered if r.get("tool") == tool]
            theirs = [r for r in remote if r.get("tool") == tool]
            check(mine and all(r.get("unit") in (unit, unit + "s") for r in mine),
                  f"bashy offer usage on C meters {tool} in {unit}")
            if theirs:
                check(sum(r.get("amount", 0) for r in theirs) ==
                      sum(r.get("amount", 0) for r in mine if r.get("principal") == "peer:o"),
                      f"bashy remote usage on O agrees with C for {tool}")

    # Unshare: the folder offer is gone (403).
    cli(c, "files", "unshare", "docs")
    status, _, _ = member(A, "files-docs", "/offers/files-docs", body={"args": {"op": "ls"}})
    check(status == 403, f"a read after unshare is refused: HTTP {status}")


def main():
    os.environ.update(PATH="/fixture:/tools:/go/bin:/python/bin:/usr/bin:/bin",
                      BASHY_HOME=str(C_HOME), BASHY_HINTS="off", BASHY_LLM_PORT=str(C_PORT),
                      GIT_TERMINAL_PROMPT="0", USER="bashy", LOGNAME="bashy",
                      GOMAXPROCS="4", GOTOOLCHAIN="local", GOPROXY="off", GOWORK="off",
                      OUTPOST_ADMIN_ADDR=ADMIN, OUTPOST_INGRESS_TRUST_LOOPBACK="1")
    started = time.monotonic()
    c, o = as_host(C_HOME, C_PORT), as_host(O_HOME, O_PORT)
    cache = HOME / ".cache/bashy/adapters/yoke"
    cache.mkdir(parents=True, exist_ok=True)
    shutil.copy("/tools/delegate", cache / "delegate")
    check(shutil.which("codex") == "/fixture/codex", "the one-shot agent stub is on PATH")
    for server, port in ((Satellite, 18881), (FakeModel, MODEL_PORT)):
        httpd = ThreadingHTTPServer(("127.0.0.1", port), server)
        threading.Thread(target=httpd.serve_forever, daemon=True).start()
    run("bashy", "tools", "set", "codex", "--set", "cli.binary=/fixture/codex")
    cli(c, "model", "add", MODEL, "--provider", "openai-compat", "--kind", "local",
        "--base-url", f"http://127.0.0.1:{MODEL_PORT}/v1", "--upstream", MODEL)
    cli(c, "agent", "add", AGENT, "--tool", "codex", "--model", MODEL)
    for record in (TRANSCRIBE, UNTYPED):
        path = HOME / (record.split()[1] + ".yaml")
        path.write_text(record)
        cli(c, "commands", "add", str(path))
    CONFIG.parent.mkdir(parents=True, exist_ok=True)
    CONFIG.write_text(json.dumps({"agent_name": "c-host", "oauth2_email": OWNER, "portal_url": ORIGIN,
                                  "access_token": TOKEN, "token": secrets.token_hex(24),
                                  "server_addr": "127.0.0.1", "server_port": 18885, "remote_port": 18886,
                                  "local_addr": "127.0.0.1:18880", "apps": []}))
    CONFIG.chmod(0o600)
    outpost = start_outpost()
    check(GRANTED.wait(WAIT), "real outpost fetched the fake host grant copy")
    door(c, C_PORT)
    door(o, O_PORT)
    scenario(c, o, outpost)
    print(f"elapsed {time.monotonic() - started:.1f}s", flush=True)
    print("PASS taas a-o-c", flush=True)


if __name__ == "__main__":
    main()
