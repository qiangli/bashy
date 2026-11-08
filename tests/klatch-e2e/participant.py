"""Klatch participant journey for the klatch-e2e sandbox: helper CLI, sealed
messaging, attachments and the canonical Bash# wrapper.

This module is an imported helper, not a runner. `run.py --suite participant`
owns the sandbox: it starts a disposable Klatch container with `bashy oci run`
(topology none), waits for health, calls `run_journey(args, url)` against that
container's URL, then removes the container, which drops the fresh store and
every account the journey created. Nothing here starts, builds or stops a
server, and the journey refuses a server whose store is already claimed, so it
can never touch a live service.

Against the container the journey provisions two fresh accounts through the
existing `examples/klatch/provision.py` admin client, seals both
(`seal_setup.mjs`), then drives:

- the internal Node helper (`app/client/cli/dist/main.js`, frozen contract in
  that directory's README) for login/status/logout, seal unlock, contacts,
  sealed conversations, members, messages, attachments and watch;
- the canonical Bash# wrapper (`examples/klatch/klatch.bsh`, run as
  `bashy --bashsharp klatch.bsh VERB ...`) for stdin passthrough, key=value
  shaping, the `post-file` workflow and argv secret refusal.

The federation domain stays empty, so both accounts are same-node contacts and
nothing leaves the sandbox. The journey's private 0700 working tree (sessions,
inputs, downloads) is removed afterwards unless --keep-tree.

No password, seal password or app token is ever placed in argv; secrets go to
the helper on stdin or in private input files, and every helper/wrapper
transcript is scanned for leaked secrets. File-mode checks (0600 sessions and
downloads) and the SIGTERM exit-status check are POSIX-only; on Windows they are
recorded under `skipped_checks`, never counted as passed.
"""
import base64
import hashlib
import json
import os
import secrets
import shutil
import signal
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent
POSIX = os.name != "nt"

STEPS = ("provision", "login", "seal", "contacts", "conversation", "messages",
         "attachments", "watch", "wrapper", "logout")

ASSISTANT = "E2E Agent"
PEER_ASSISTANT = "E2E Peer"
WRAPPER_ASSISTANT = "Wrapper Bot"

WRAPPER_BLOCKED = (
    "wrapper has no participant verbs on this checkout "
    "(owning story 296fa5066f54, Bash# Klatch participant surface)"
)


class Blocked(Exception):
    pass


class Check(Exception):
    """A participant assertion failed."""


def _bashy_bin():
    return os.environ.get("BASHY_BIN", "bashy")


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 16), b""):
            h.update(chunk)
    return h.hexdigest()


def sha256_tree(root):
    """Digest of every file under root (relative path + content), sorted; main.js alone
    does not change when only a sibling module (e.g. commands.js) does."""
    h = hashlib.sha256()
    for p in sorted(Path(root).rglob("*")):
        if p.is_file():
            h.update(p.relative_to(root).as_posix().encode() + b"\0" + sha256_file(p).encode() + b"\n")
    return h.hexdigest()


def private_write(path, data):
    """Write bytes/str to a new 0600 file (never follows or replaces a link)."""
    if isinstance(data, str):
        data = data.encode()
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
                 | getattr(os, "O_BINARY", 0), 0o600)
    with os.fdopen(fd, "wb") as f:
        f.write(data)
    return path


def is_private(path):
    return os.stat(path).st_mode & 0o777 == 0o600


def load_provision():
    """The existing examples/klatch admin client (single source of the admin API)."""
    import importlib.util
    spec = importlib.util.spec_from_file_location(
        "klatch_provision", REPO / "examples" / "klatch" / "provision.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def wrapper_has_participant_verbs(path):
    try:
        text = Path(path).read_text(encoding="utf-8")
    except OSError:
        return False
    return "post-file" in text and "messages-send" in text and "KLATCH_HELPER" in text


def leaked(text, secret_values):
    """Names (never values) of secrets appearing in a transcript."""
    return [name for name, value in secret_values.items() if value and value in (text or "")]


def summarize(steps):
    statuses = [steps[name]["status"] for name in STEPS]
    if "fail" in statuses:
        return "FAIL"
    if "blocked" in statuses or "pending" in statuses:
        return "BLOCKED"
    return "PASS"


class Journey:
    """The participant journey's private working tree against one sandbox URL."""

    def __init__(self, args, url):
        self.args = args
        self.url = url.rstrip("/")
        self.root = Path(tempfile.mkdtemp(prefix="klatch-participant-e2e-"))
        os.chmod(self.root, 0o700)
        self.secrets = {}
        self.accounts = {}
        self.calls = 0
        self.skipped = []
        self._serial = 0

    # ---------- files ----------
    def path(self, name):
        return str(self.root / name)

    def input_file(self, obj):
        self._serial += 1
        return private_write(self.path(f"input-{self._serial}.json"), json.dumps(obj))

    def posix_check(self, name, ok, message):
        """A file-mode style check: asserted on POSIX, recorded as skipped elsewhere."""
        if not POSIX:
            if name not in self.skipped:
                self.skipped.append(name)
            return None
        if not ok():
            raise Check(message)
        return name

    # ---------- sandbox ----------
    def admin_status(self):
        with urllib.request.urlopen(self.url + "/admin/status", timeout=10) as r:
            return json.loads(r.read().decode() or "null")

    def provision(self, handles):
        status = self.admin_status()
        if status is not True:
            # a claimed store is not a disposable sandbox; never provision into it
            raise Blocked(f"server store is already claimed (admin/status={status!r}); "
                          "the participant journey only runs against a fresh sandbox container")
        mod = load_provision()
        client = mod.Client(self.url, resolve=lambda ref: ref)
        admin = secrets.token_urlsafe(18)
        self.secrets["admin_password"] = admin
        try:
            client.setup_admin(admin)
            session = client.admin_session(admin)
            # empty domain: same-node contacts, no federation hop out of the sandbox
            client.set_config(session, "")
            for handle in handles:
                password = secrets.token_urlsafe(18)
                self.secrets[f"{handle}_password"] = password
                client.create_account(session, handle, password)
                self.accounts[handle] = {"handle": handle, "password": password}
            listed = sorted(mod.account_handle(a) for a in client.accounts(session))
        except SystemExit as e:  # provision.py reports HTTP failures via SystemExit
            raise RuntimeError(f"sandbox provisioning: {e}")
        if listed != sorted(handles):
            raise Check(f"provisioned accounts {listed} != {sorted(handles)}")
        return {"accounts": len(listed)}

    def close(self):
        if getattr(self.args, "keep_tree", False):
            return {"status": "done", "detail": f"kept journey tree {self.root}"}
        shutil.rmtree(self.root, ignore_errors=True)
        if self.root.exists():
            return {"status": "failed", "detail": "journey tree removal incomplete"}
        return {"status": "done", "detail": "removed journey tree"}

    # ---------- raw API (sealing proof only; reads the journey's own session) ----------
    def raw(self, who, path, query=None):
        token = json.loads(Path(self.session(who)).read_text())["token"]
        q = dict(query or {}, agent=token)
        with urllib.request.urlopen(f"{self.url}{path}?{urllib.parse.urlencode(q)}", timeout=30) as r:
            return r.read()

    def session(self, who):
        return self.path(f"session-{who}.json")

    # ---------- helper / wrapper ----------
    def _env(self):
        env = dict(os.environ, BASHY_HINTS="off")
        node_dir = os.path.dirname(self.args.node_path) if self.args.node_path else ""
        if node_dir:
            env["PATH"] = node_dir + os.pathsep + env.get("PATH", "")
        return env

    def _record(self, argv, p):
        self.calls += 1
        names = leaked((p.stdout or "") + (p.stderr or "") + " ".join(argv), self.secrets)
        if names:
            raise Check(f"secret leaked in {argv[-3:]} transcript: {names}")

    @staticmethod
    def _envelope(p):
        stream = p.stdout if p.returncode == 0 else p.stderr
        lines = [ln for ln in (stream or "").splitlines() if ln.strip()]
        try:
            return json.loads(lines[-1]) if lines else None
        except ValueError:
            return None

    def _run(self, argv, stdin, timeout, env):
        return subprocess.run(argv, input=stdin, capture_output=True, text=True, encoding="utf-8",
                              errors="replace", timeout=timeout, env=env)

    def helper(self, verb, who, payload=None, input_file=False, expect=None):
        """Run one helper verb as `who`. Returns the result, or the error code when expect is set."""
        argv = [self.args.node, self.args.helper, verb, "--session", self.session(who), "--json"]
        stdin = json.dumps(payload or {})
        if input_file:
            argv += ["--input", self.input_file(payload or {})]
            stdin = ""
        p = self._run(argv, stdin, 120, self._env())
        self._record(argv, p)
        return self._outcome(f"helper {verb}", p, expect)

    def wrapper(self, args, who, payload=None, env=None, expect=None):
        argv = [_bashy_bin(), "--bashsharp", self.args.wrapper] + list(args)
        if who:
            argv += ["--session", self.session(who)]
        e = self._env()
        e.update(KLATCH_HELPER=self.args.helper)
        e.update(env or {})
        p = self._run(argv, json.dumps(payload) if payload is not None else "", 180, e)
        self._record(argv, p)
        return self._outcome(f"wrapper {args[0]}", p, expect)

    def _outcome(self, what, p, expect):
        env = self._envelope(p)
        if expect is not None:
            code = ((env or {}).get("error") or {}).get("code")
            if p.returncode == 0 or code != expect:
                raise Check(f"{what}: expected failure {expect}, got exit {p.returncode} code {code!r}")
            return code
        if p.returncode != 0 or not env or env.get("ok") is not True:
            code = ((env or {}).get("error") or {}).get("code")
            raise Check(f"{what}: exit {p.returncode} code {code!r}")
        return env.get("result")


# ---------- shape helpers over helper results ----------
def card_for(cards, guid):
    for c in cards or []:
        prof = (c.get("data") or {}).get("cardProfile") or {}
        if prof.get("guid") == guid:
            return c
    return None


def card_status(card):
    return ((card or {}).get("data") or {}).get("cardDetail", {}).get("status")


def topic_text(topic):
    return ((((topic or {}).get("data") or {}).get("topicDetail") or {}).get("data") or {}).get("text")


def topic_guid(topic):
    return (((topic or {}).get("data") or {}).get("topicDetail") or {}).get("guid")


def find_topic(topics, topic_id):
    return next((t for t in topics or [] if t.get("id") == topic_id), None)


# ---------- steps ----------
def step_provision(sb, ctx):
    info = sb.provision([ctx["a"], ctx["b"]])
    return info, ["sandbox_store_fresh", "accounts_provisioned"]


def step_login(sb, ctx):
    a, b = ctx["a"], ctx["b"]
    checks = []
    sb.helper("auth-login", "bad", {"server": sb.url, "username": a, "password": "wrong-" + secrets.token_hex(4)},
              input_file=True, expect="AUTH_REQUIRED")
    checks.append("wrong_password_refused")
    for who in (a, b):
        r = sb.helper("auth-login", who, {"server": sb.url, "username": who,
                                          "password": sb.accounts[who]["password"]}, input_file=True)
        if not r.get("guid"):
            raise Check(f"auth-login {who}: no guid")
        ctx["guid"][who] = r["guid"]
        private = sb.posix_check("session_0600", lambda w=who: is_private(sb.session(w)),
                                 f"session file for {who} is not 0600")
        sb.secrets[f"{who}_token"] = json.loads(Path(sb.session(who)).read_text())["token"]
        st = sb.helper("auth-status", who)
        if st.get("guid") != r["guid"] or st.get("server") != sb.url:
            raise Check(f"auth-status {who} does not match login")
    if ctx["guid"][a] == ctx["guid"][b]:
        raise Check("two accounts share one guid")
    checks += ["login_input_file", private, "auth_status"]
    return {"guids_distinct": True}, checks


def step_seal(sb, ctx):
    checks = []
    for who in (ctx["a"], ctx["b"]):
        pw = secrets.token_urlsafe(16)
        sb.secrets[f"{who}_seal"] = pw
        ctx["seal"][who] = pw
        argv = [sb.args.node, str(HERE / "seal_setup.mjs"), "--session", sb.session(who),
                "--input", sb.input_file({"sealPassword": pw})]
        p = sb._run(argv, "", 120, sb._env())
        sb._record(argv, p)
        r = sb._outcome("seal_setup", p, None)
        if not r.get("created"):
            raise Check(f"seal_setup {who}: fresh account reported already sealed")
        u = sb.helper("seal-unlock", who, {"sealPassword": pw})
        if not u.get("unlocked") or u.get("publicKey") != r.get("publicKey"):
            raise Check(f"seal-unlock {who}: key mismatch")
        sb.helper("seal-unlock", who, {"sealPassword": pw + "x"}, expect="SEAL_UNLOCK_FAILED")
        st = sb.helper("auth-status", who)
        if st.get("sealable") is not True:
            raise Check(f"auth-status {who}: account not sealable")
    checks += ["seal_created", "seal_unlock", "wrong_seal_fails_closed"]
    return {"sealed_accounts": 2}, checks


def step_contacts(sb, ctx):
    a, b = ctx["a"], ctx["b"]
    inv = sb.helper("contacts-invite", a, {"guid": ctx["guid"][b]})
    pending = card_for(sb.helper("contacts-list", b), ctx["guid"][a])
    if not pending:
        raise Check("invitee has no card for the inviter")
    acc = sb.helper("contacts-accept", b, {"cardId": pending["id"]})
    if acc.get("status") != "connected":
        raise Check(f"contacts-accept status {acc.get('status')!r}")
    ca = card_for(sb.helper("contacts-list", a), ctx["guid"][b])
    cb = card_for(sb.helper("contacts-list", b), ctx["guid"][a])
    if card_status(ca) != "connected" or card_status(cb) != "connected":
        raise Check(f"cards not connected: {card_status(ca)!r}/{card_status(cb)!r}")
    if ca["id"] != inv.get("cardId"):
        raise Check("contacts-invite cardId is not the listed card")
    for card in (ca, cb):
        if "token" in json.dumps(card).lower():
            raise Check("contacts-list exposes a token-named field")
    ctx["card"][a], ctx["card"][b] = ca["id"], cb["id"]
    return {"invite_status": inv.get("status"), "accepted": acc.get("status")}, \
        ["invite", "accept", "mutual_connected", "tokens_redacted"]


def step_conversation(sb, ctx):
    a, b = ctx["a"], ctx["b"]
    checks = []
    title = f"sealed e2e {ctx['stamp']}"
    sb.helper("conversations-create", a, {"title": title, "cards": [ctx["card"][a]]}, expect="SEAL_LOCKED")
    checks.append("sealed_by_default_needs_seal")
    ch = sb.helper("conversations-create", a, {"title": title, "cards": [ctx["card"][a]],
                                               "sealPassword": ctx["seal"][a]})
    cid = ch.get("id")
    if not cid:
        raise Check("conversations-create returned no id")
    ctx["channel"] = cid
    raw = json.loads(sb.raw(a, f"/content/channels/{cid}/detail"))
    if raw.get("dataType") != "sealed" or title in json.dumps(raw):
        raise Check("conversation is not sealed on the wire")
    checks.append("wire_sealed_subject")
    peer = {"cardId": ctx["card"][b], "sealPassword": ctx["seal"][b]}
    listed = sb.helper("conversations-list", b, peer)
    if not any(c.get("id") == cid for c in listed or []):
        raise Check("member does not list the sealed conversation")
    shown = sb.helper("conversations-show", b, dict(peer, channelId=cid))
    if (shown.get("data") or {}).get("subject") != title:
        raise Check("member cannot decrypt the sealed subject")
    sb.helper("conversations-show", b, {"cardId": ctx["card"][b], "channelId": cid}, expect="SEAL_LOCKED")
    checks += ["member_lists", "member_decrypts_subject", "member_without_seal_locked"]
    members = sb.helper("members-list", a, {"channelId": cid, "sealPassword": ctx["seal"][a]})
    if ctx["card"][a] not in json.dumps(members):
        raise Check("members-list does not include the invited card")
    checks.append("members_list")
    renamed = title + " renamed"
    sb.helper("conversations-rename", a, {"channelId": cid, "title": renamed, "sealPassword": ctx["seal"][a]})
    shown = sb.helper("conversations-show", b, dict(peer, channelId=cid))
    if (shown.get("data") or {}).get("subject") != renamed:
        raise Check("member does not see the sealed rename")
    sb.helper("conversations-rename", b, dict(peer, channelId=cid, title="hijack"), expect="OWNER_REQUIRED")
    checks += ["sealed_rename", "rename_owner_only"]
    return {"channelId": cid}, checks


def step_messages(sb, ctx):
    a, b, cid = ctx["a"], ctx["b"], ctx["channel"]
    own = {"channelId": cid, "sealPassword": ctx["seal"][a]}
    peer = {"channelId": cid, "cardId": ctx["card"][b], "sealPassword": ctx["seal"][b]}
    checks = []
    sb.helper("messages-send", a, dict(own, text="no attribution"), expect="INVALID_INPUT")
    checks.append("attribution_required")
    text = f"sealed hello {ctx['stamp']}"
    t1 = sb.helper("messages-send", a, dict(own, text=text, assistant=ASSISTANT)).get("id")
    if not t1:
        raise Check("messages-send returned no topic id")
    ctx["topic"] = t1
    wire = sb.raw(a, f"/content/channels/{cid}/topics").decode()
    if text in wire or "sealedtopic" not in wire:
        raise Check("message is not sealed on the wire")
    checks.append("wire_sealed_message")
    got = find_topic(sb.helper("messages-list", b, peer), t1)
    if topic_text(got) != f"[Assistant: {ASSISTANT}] {text}" or topic_guid(got) != ctx["guid"][a]:
        raise Check("member does not read the attributed sealed message")
    sb.helper("messages-list", b, {"channelId": cid, "cardId": ctx["card"][b]}, expect="SEAL_LOCKED")
    sb.helper("messages-list", b, dict(peer, sealPassword=ctx["seal"][b] + "x"), expect="SEAL_UNLOCK_FAILED")
    checks += ["member_decrypts", "author_guid", "no_seal_locked", "wrong_seal_fails_closed"]
    reply = f"sealed reply {ctx['stamp']}"
    t2 = sb.helper("messages-send", b, dict(peer, text=reply, assistant=PEER_ASSISTANT)).get("id")
    got = find_topic(sb.helper("messages-list", a, own), t2)
    if topic_text(got) != f"[Assistant: {PEER_ASSISTANT}] {reply}" or topic_guid(got) != ctx["guid"][b]:
        raise Check("owner does not read the member's attributed reply")
    checks.append("member_reply_attributed")
    edited = text + " (edited)"
    sb.helper("messages-edit", a, dict(own, topicId=t1, text=edited, assistant=ASSISTANT))
    shown = sb.helper("messages-show", b, dict(peer, topicId=t1))
    if (shown.get("data") or {}).get("text") != f"[Assistant: {ASSISTANT}] {edited}":
        raise Check("member does not see the edit")
    sb.helper("messages-edit", b, dict(peer, topicId=t1, text="hijack", assistant=PEER_ASSISTANT),
              expect="AUTHOR_REQUIRED")
    sb.helper("messages-delete", b, dict(peer, topicId=t1), expect="AUTHOR_REQUIRED")
    checks += ["edit_reattributed", "edit_author_only", "delete_author_only"]
    sb.helper("messages-delete", b, dict(peer, topicId=t2))
    if find_topic(sb.helper("messages-list", a, own), t2):
        raise Check("deleted message still listed")
    checks.append("delete")
    return {"topics": 2}, checks


def step_attachments(sb, ctx):
    a, b, cid, t1 = ctx["a"], ctx["b"], ctx["channel"], ctx["topic"]
    payload = os.urandom(70 * 1024 + 13)
    src = private_write(sb.path("payload.bin"), payload)
    up = sb.helper("attachments-upload", a, {"channelId": cid, "topicId": t1, "path": src,
                                             "text": "sealed file", "assistant": ASSISTANT,
                                             "sealPassword": ctx["seal"][a]})
    parts = (((up or {}).get("asset") or {}).get("encrypted") or {}).get("parts") or []
    if not parts or not parts[0].get("partId"):
        raise Check("sealed upload did not produce an encrypted asset part")
    asset = parts[0]["partId"]
    blob = sb.raw(a, f"/content/channels/{cid}/topics/{t1}/assets/{asset}")
    if payload[:64] in blob or base64.b64encode(payload[:48]) in blob:
        raise Check("attachment is not sealed at rest")
    peer = {"channelId": cid, "topicId": t1, "assetId": asset, "cardId": ctx["card"][b],
            "sealPassword": ctx["seal"][b]}
    dst = sb.path("download.bin")
    got = sb.helper("attachments-download", b, dict(peer, path=dst))
    if got.get("bytes") != len(payload) or Path(dst).read_bytes() != payload:
        raise Check("downloaded bytes differ from upload")
    private = sb.posix_check("download_private", lambda: is_private(dst), "download is not private")
    sb.helper("attachments-download", b, dict(peer, path=dst), expect="OPERATION_FAILED")
    shown = sb.helper("messages-show", b, {"channelId": cid, "topicId": t1, "cardId": ctx["card"][b],
                                           "sealPassword": ctx["seal"][b]})
    if (shown.get("data") or {}).get("text") != f"[Assistant: {ASSISTANT}] sealed file":
        raise Check("attachment caption not attributed")
    sb.helper("attachments-upload", b, {"channelId": cid, "topicId": t1, "path": src, "text": "x",
                                        "assistant": PEER_ASSISTANT, "cardId": ctx["card"][b],
                                        "sealPassword": ctx["seal"][b]}, expect="AUTHOR_REQUIRED")
    return {"bytes": len(payload), "sha256": hashlib.sha256(payload).hexdigest()}, \
        ["sealed_upload", "sealed_at_rest", "byte_roundtrip", private,
         "no_overwrite", "caption_attributed", "upload_author_only"]


def step_watch(sb, ctx):
    """Owner watches its sealed conversation; a member post arrives as a snapshot."""
    a, b, cid = ctx["a"], ctx["b"], ctx["channel"]
    argv = [sb.args.node, sb.args.helper, "watch", "--session", sb.session(a), "--json"]
    proc = subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            text=True, encoding="utf-8", errors="replace", env=sb._env())
    proc.stdin.write(json.dumps({"channelId": cid, "sealPassword": ctx["seal"][a]}))
    proc.stdin.close()
    lines, err = [], []
    reader = threading.Thread(target=lambda: lines.extend(iter(proc.stdout.readline, "")), daemon=True)
    ereader = threading.Thread(target=lambda: err.extend(iter(proc.stderr.readline, "")), daemon=True)
    reader.start()
    ereader.start()
    text = f"watched {ctx['stamp']}"
    want = f"[Assistant: {PEER_ASSISTANT}] {text}"

    def events():
        return [json.loads(ln) for ln in list(lines) if ln.strip()]

    try:
        deadline = time.time() + 30
        while not any(e.get("result", {}).get("event") == "snapshot" for e in events()):
            if proc.poll() is not None or time.time() > deadline:
                raise Check(f"watch produced no initial snapshot (exit {proc.poll()})")
            time.sleep(0.2)
        sb.helper("messages-send", b, {"channelId": cid, "cardId": ctx["card"][b],
                                       "sealPassword": ctx["seal"][b], "text": text,
                                       "assistant": PEER_ASSISTANT})
        deadline = time.time() + 30
        while True:
            snaps = [e["result"]["result"] for e in events() if e.get("result", {}).get("event") == "snapshot"]
            if any(topic_text(t) == want for s in snaps for t in s or []):
                break
            if proc.poll() is not None or time.time() > deadline:
                raise Check("watch did not deliver the member's sealed post")
            time.sleep(0.2)
    finally:
        if proc.poll() is None:
            proc.send_signal(signal.SIGTERM)
        try:
            rc = proc.wait(timeout=15)
        except subprocess.TimeoutExpired:
            proc.kill()
            rc = proc.wait(timeout=15)
        reader.join(timeout=5)
        ereader.join(timeout=5)
    names = leaked("".join(lines) + "".join(err), sb.secrets)
    if names:
        raise Check(f"secret leaked in watch transcript: {names}")
    # Windows has no SIGTERM delivery (send_signal terminates the process)
    clean = sb.posix_check("sigterm_clean_exit", lambda: rc == 0,
                           f"watch did not exit cleanly on SIGTERM (exit {rc})")
    if not all(e.get("ok") is True for e in events()):
        raise Check("watch emitted a non-envelope line")
    statuses = [e["result"].get("status") for e in events() if e["result"].get("event") == "status"]
    return {"events": len(events()), "statuses": statuses}, \
        ["ndjson_envelopes", "initial_snapshot", "live_sealed_update", clean]


def step_wrapper(sb, ctx):
    if not os.path.isfile(sb.args.wrapper):
        raise Blocked(f"wrapper not found: {sb.args.wrapper}")
    if not wrapper_has_participant_verbs(sb.args.wrapper):
        raise Blocked(WRAPPER_BLOCKED)
    a, b, cid = ctx["a"], ctx["b"], ctx["channel"]
    checks = []
    st = sb.wrapper(["auth-status"], a)
    if st.get("guid") != ctx["guid"][a]:
        raise Check("wrapper auth-status does not match the session")
    checks.append("passthrough_status")
    sb.wrapper(["messages-send", f"channelId={cid}", "sealPassword=x"], a, expect="SECRET_VIA_ARGV")
    checks.append("argv_secret_refused")
    # sealed send: secrets travel on stdin through the wrapper
    text = f"wrapper sealed {ctx['stamp']}"
    t = sb.wrapper(["messages-send"], a, {"channelId": cid, "text": text, "assistant": WRAPPER_ASSISTANT,
                                          "sealPassword": ctx["seal"][a]}).get("id")
    got = find_topic(sb.helper("messages-list", b, {"channelId": cid, "cardId": ctx["card"][b],
                                                    "sealPassword": ctx["seal"][b]}), t)
    if topic_text(got) != f"[Assistant: {WRAPPER_ASSISTANT}] {text}":
        raise Check("member does not read the wrapper's sealed send")
    checks.append("sealed_send_stdin")
    # shaping: key=value fields plus KLATCH_ASSISTANT on an explicitly unsealed conversation
    plain = sb.wrapper(["conversations-create", f"title=plain e2e {ctx['stamp']}",
                        f"cards:={json.dumps([ctx['card'][a]])}", "sealed:=false"], a).get("id")
    shaped = f"wrapper shaped {ctx['stamp']}"
    t2 = sb.wrapper(["messages-send", f"channelId={plain}", f"text={shaped}"], a,
                    env={"KLATCH_ASSISTANT": WRAPPER_ASSISTANT}).get("id")
    got = find_topic(sb.wrapper(["messages-list", f"channelId={plain}", f"cardId={ctx['card'][b]}"], b), t2)
    if topic_text(got) != f"[Assistant: {WRAPPER_ASSISTANT}] {shaped}":
        raise Check("shaped send not attributed through KLATCH_ASSISTANT")
    checks += ["shaped_create_unsealed", "shaped_send_default_assistant", "shaped_list_contact_scope"]
    # post-file workflow on the sealed conversation
    payload = os.urandom(9000)
    src = private_write(sb.path("wrapper-payload.bin"), payload)
    pf = sb.wrapper(["post-file"], a, {"channelId": cid, "text": "wrapper file", "assistant": WRAPPER_ASSISTANT,
                                       "path": src, "sealPassword": ctx["seal"][a]})
    topic = pf.get("topicId")
    parts = ((((pf.get("upload") or {}).get("asset") or {}).get("encrypted") or {}).get("parts")) or []
    if not topic or not parts:
        raise Check("post-file did not return a topic and an encrypted asset")
    dst = sb.path("wrapper-download.bin")
    sb.helper("attachments-download", b, {"channelId": cid, "topicId": topic, "assetId": parts[0]["partId"],
                                          "path": dst, "cardId": ctx["card"][b], "sealPassword": ctx["seal"][b]})
    if Path(dst).read_bytes() != payload:
        raise Check("post-file attachment bytes differ")
    checks.append("post_file_sealed_roundtrip")
    return {"wrapper_sha256": sha256_file(sb.args.wrapper)}, checks


def revocation_answer(status_before, fetch):
    """How the server answers a logged-out app token: "refused" or "empty".

    fetch() returns the /account/status body for the revoked token or raises
    HTTPError. This server's session lookup (GORM Find) answers 200 with an
    empty session for an unknown token, so an HTTP refusal and a 200 that no
    longer resolves the account (its seal key is gone) both prove revocation;
    a 200 still carrying the account's seal key does not.
    """
    key = ((status_before or {}).get("seal") or {}).get("publicKey")
    try:
        after = fetch()
    except urllib.error.HTTPError as e:
        if e.code in (401, 403, 404):
            return "refused"
        raise Check(f"revoked token answered HTTP {e.code}")
    if not key:
        raise Check("cannot prove revocation: the account status had no seal key before logout")
    if ((after or {}).get("seal") or {}).get("publicKey") == key:
        raise Check("revoked app token still resolves the account")
    return "empty"


def step_logout(sb, ctx):
    a = ctx["a"]
    token = sb.secrets.get(f"{a}_token")

    def status():
        url = f"{sb.url}/account/status?agent={urllib.parse.quote(token)}"
        with urllib.request.urlopen(url, timeout=10) as r:
            return json.loads(r.read().decode() or "null")

    before = status()
    for who in (a, ctx["b"]):
        r = sb.helper("auth-logout", who)
        if r.get("loggedOut") is not True or os.path.exists(sb.session(who)):
            raise Check(f"auth-logout {who} left a session")
    answer = revocation_answer(before, status)
    return {"revoked_token_answer": answer}, ["logout_removes_session", "token_revoked"]


STEP_FUNCS = {
    "provision": step_provision, "login": step_login, "seal": step_seal, "contacts": step_contacts,
    "conversation": step_conversation, "messages": step_messages, "attachments": step_attachments,
    "watch": step_watch, "wrapper": step_wrapper, "logout": step_logout,
}
# A failed or blocked step stops the journey (later steps need its state),
# except these, whose outcome does not invalidate the steps after them.
INDEPENDENT = {"watch", "wrapper"}


def preflight(args):
    """Concrete BLOCKED reason for a missing prerequisite, checked before run.py starts a container."""
    if not args.node_path:
        return f"node not found ({args.node}); Node >= 22 is a participant MVP dependency"
    if not args.helper or not os.path.isfile(args.helper):
        return (f"participant helper not built: {args.helper or '(unset)'} "
                "(npm ci && npm run build in klatch app/client/cli; pass --helper or set KLATCH_HELPER)")
    return None


def run_journey(args, url, step_funcs=None, journey_factory=Journey):
    """Run every participant step against the sandbox at url.

    Returns {"steps", "checks_passed", "skipped_checks", "cli_calls",
    "accounts", "journey_teardown"}. Never raises: a failure is a failed step.
    """
    step_funcs = dict(STEP_FUNCS, **(step_funcs or {}))
    stamp = f"{time.strftime('%Y%m%d%H%M%S')}{os.getpid()}"
    ctx = {"a": f"e2ea{secrets.token_hex(3)}", "b": f"e2eb{secrets.token_hex(3)}", "stamp": stamp,
           "guid": {}, "seal": {}, "card": {}}
    steps = {name: {"status": "pending"} for name in STEPS}
    try:
        jr = journey_factory(args, url)
    except Exception as e:  # noqa: BLE001 - no tree, no journey
        steps = {name: {"status": "fail", "reason": f"journey setup: {e}"} for name in STEPS}
        return {"steps": steps, "checks_passed": 0, "skipped_checks": [], "cli_calls": 0,
                "accounts": [ctx["a"], ctx["b"]],
                "journey_teardown": {"status": "skipped", "detail": "no journey tree"}}
    try:
        stop_reason = None
        for name in STEPS:
            # logout doubles as session cleanup: it runs whenever login passed
            runnable = not stop_reason or (name == "logout" and steps["login"]["status"] == "pass")
            if not runnable:
                steps[name] = {"status": "blocked", "reason": stop_reason}
                continue
            t0 = time.time()
            try:
                detail, checks = step_funcs[name](jr, ctx)
                steps[name] = {"status": "pass", "checks": [c for c in checks if c], "detail": detail}
            except Blocked as e:
                steps[name] = {"status": "blocked", "reason": str(e)}
                if name not in INDEPENDENT:
                    stop_reason = stop_reason or f"skipped: {name} blocked"
            except Exception as e:  # noqa: BLE001 - any failure is a failed leg
                steps[name] = {"status": "fail", "reason": f"{type(e).__name__}: {e}"}
                if name not in INDEPENDENT:
                    stop_reason = stop_reason or f"skipped: {name} failed"
            steps[name]["seconds"] = round(time.time() - t0, 2)
    finally:
        try:
            closed = jr.close()
        except Exception as e:  # noqa: BLE001 - teardown records, never raises
            closed = {"status": "failed", "detail": str(e)}
    return {
        "steps": steps,
        "checks_passed": sum(len(s.get("checks", [])) for s in steps.values()),
        "skipped_checks": list(getattr(jr, "skipped", [])),
        "cli_calls": getattr(jr, "calls", 0),
        "accounts": [ctx["a"], ctx["b"]],
        "journey_teardown": closed,
    }


def candidate(args):
    info = {}
    for key, path in (("helper_sha256", args.helper), ("wrapper_sha256", args.wrapper)):
        try:
            info[key] = sha256_file(path)
        except (OSError, TypeError):
            info[key] = "unknown"
    try:
        info["helper_dist_sha256"] = sha256_tree(Path(args.helper).parent) if args.helper else "unknown"
    except OSError:
        info["helper_dist_sha256"] = "unknown"
    try:
        p = subprocess.run([args.node, "--version"], capture_output=True, text=True, timeout=30)
        info["node_version"] = (p.stdout or p.stderr).strip().splitlines()[0] if p.returncode == 0 else "unknown"
    except Exception:  # noqa: BLE001 - provenance is best-effort
        info["node_version"] = "unknown"
    info["helper_rev"] = getattr(args, "helper_rev", None) or os.environ.get("KLATCH_HELPER_REV") or "unknown"
    return info


def add_arguments(ap):
    """The participant options run.py exposes for --suite participant."""
    g = ap.add_argument_group("participant suite")
    g.add_argument("--helper", default=os.environ.get("KLATCH_HELPER"),
                   help="participant helper dist/main.js (also KLATCH_HELPER)")
    g.add_argument("--helper-rev", default=None,
                   help="klatch commit the helper was built from, for evidence (also KLATCH_HELPER_REV)")
    g.add_argument("--node", default=os.environ.get("NODE", "node"), help="Node >= 22 executable")
    g.add_argument("--wrapper", default=str(REPO / "examples" / "klatch" / "klatch.bsh"),
                   help="Bash# participant wrapper (default: examples/klatch/klatch.bsh)")
    g.add_argument("--keep-tree", action="store_true",
                   help="keep the journey's private tree (contains secrets) for debugging")
    return ap


def resolve_args(args):
    node = getattr(args, "node", None) or "node"
    found = shutil.which(node)
    args.node_path = found
    if found:
        args.node = found
    return args
