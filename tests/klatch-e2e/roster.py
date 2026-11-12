"""Klatch roster journey for the klatch-e2e sandbox: the SSO roster endpoint.

This module is an imported helper, not a runner. `run.py --suite roster` owns
the sandbox: it starts a disposable Klatch container with `bashy oci run`
(topology none) carrying `-e KLATCH_SSO_SECRET=...`, waits for health, calls
`run_journey(args, url, secret)` against that container's URL, then removes
the container, which drops the fresh store and every account the journey
created. Nothing here starts, builds or stops a server, and the journey
refuses a store that is already claimed, so it can never touch a live
service.

Against the container the journey bootstraps the admin through the existing
`examples/klatch/provision.py` admin client (the single source of the admin
API) with an empty federation domain, then drives the operator roster:

- push a roster for the owner plus one member through
  `PUT /admin/sso/roster` (`Client.set_roster`, the same call
  `bashy klatch sync` makes) and assert the reconciliation counts;
- push the same roster again and assert the no-op counts (idempotent);
- sign roster members in through the outpost single sign-on route
  (`POST /account/sso` with the HMAC-signed identity headers, then the
  one-time code through `PUT /account/access`) and assert unlisted emails
  and unsigned requests are refused;
- assert the Team channel is owned by the roster owner (the owner's own
  channel listing carries it, the member's does not) and that the roster
  meshed the members into mutual connected contacts;
- remove the member and assert the removal counts, the dropped contacts,
  the refused sign-in and the disable itself (re-adding the member reports
  `enabled: 1`, which only happens for an account that existed disabled);
- assert an external-node contact is refused (no federation domain): a fully
  signed identity message carrying a foreign node is built here with the
  standard library only (RSA PKCS#1 v1.5 over the SHA-256 digest, exactly
  what the server's ReadDataMessage verifies) and must be answered 405.

The roster endpoint shipped in klatch `5334662c`; the released 1.2.7 image
predates it. The journey probes the endpoint before anything else and is
BLOCKED (never a pass, never a skip) with the local-build instructions when
the image lacks it.

No admin password or SSO secret is ever placed in argv; both travel in
query parameters and headers the server contract requires, and neither is
written to the evidence record.
"""
import base64
import hashlib
import hmac
import json
import os
import secrets
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent

STEPS = ("setup", "push", "signin", "team", "remove", "federation")

TEAM_SUBJECT = "Team"
SSO_PREFIX = "/klatch-e2e"
SSO_APP = "klatch-e2e"
SECRET_ENV = "KLATCH_E2E_SSO_SECRET"

ENDPOINT_BLOCKED = (
    "server image predates the roster endpoint (PUT /admin/sso/roster "
    "shipped in klatch 5334662c; the released 1.2.7 image does not carry "
    "it): build a local image from a klatch tree that contains it - "
    "`bashy oci build -f tests/klatch-e2e/Containerfile.server "
    "-t localhost/klatch-e2e:REV KLATCH_TREE` - and pass --image "
    "(or set KLATCH_E2E_IMAGE)"
)
STORE_BLOCKED = (
    "server store is already claimed; the roster journey only runs against "
    "a fresh sandbox container"
)


class Blocked(Exception):
    pass


class Check(Exception):
    """A roster assertion failed."""


# ---------- pure helpers (offline-testable) ----------
def roster_counts(result):
    """The four reconciliation counters of a roster PUT, missing keys as None."""
    if not isinstance(result, dict):
        return None
    return {k: result.get(k) for k in ("members", "created", "enabled", "disabled")}


def assert_counts(what, got, **want):
    """Assert a roster PUT answered exactly the wanted counters."""
    counts = roster_counts(got)
    if counts is None:
        raise Check(f"{what}: roster PUT answered no counts: {got!r}")
    missing = [k for k in ("members", "created", "enabled", "disabled") if counts.get(k) is None]
    if missing:
        raise Check(f"{what}: roster PUT answered no counts (missing {missing}): {got!r}")
    if counts != want:
        raise Check(f"{what}: counts {counts} != {dict(want)}")


def sso_headers(secret, email, role="user", prefix=SSO_PREFIX, ts=None):
    """The outpost identity headers the server's verifyOutpostIdentity checks.

    X-Outpost-Identity-Sig = hex HMAC-SHA256(secret,
      Remote-User \\n Remote-Groups \\n X-Forwarded-Prefix \\n Ts)
    """
    stamp = str(int(ts if ts is not None else time.time()))
    mac = hmac.new(secret.encode(), f"{email}\n{role}\n{prefix}\n{stamp}".encode(),
                   hashlib.sha256).hexdigest()
    return {"Remote-User": email, "Remote-Groups": role, "X-Forwarded-Prefix": prefix,
            "X-Outpost-Identity-Ts": stamp, "X-Outpost-Identity-Sig": mac}


def endpoint_present(status):
    """Classify the roster endpoint probe: None when present, a reason when not.

    The probe PUTs with an invalid token: a missing route answers 404, a
    present one answers 401 whatever the store state is.
    """
    if status == 404:
        return ENDPOINT_BLOCKED
    if status == 401:
        return None
    return f"roster endpoint probe answered HTTP {status}"


def sign_in_answer(status):
    """'refused' when the server refused the sign-in, else the status itself."""
    return "refused" if status == 403 else status


def contact_answer(status):
    """'refused' when the server refused the contact, else the status itself."""
    return "refused" if status == 405 else status


# ---------- minimal DER / RSA (standard library only) ----------
_OID_RSA = bytes([0x2A, 0x86, 0x48, 0x86, 0xF7, 0x0D, 0x01, 0x01, 0x01])  # 1.2.840.113549.1.1.1
_OID_SHA256 = bytes([0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01])  # 2.16.840.1.101.3.4.2.1


def _der_len(n):
    if n < 0x80:
        return bytes([n])
    body = n.to_bytes((n.bit_length() + 7) // 8, "big")
    return bytes([0x80 | len(body)]) + body


def _der(tag, content):
    return bytes([tag]) + _der_len(len(content)) + content


def _der_int(value):
    body = value.to_bytes(max(1, (value.bit_length() + 7) // 8), "big")
    if body[0] & 0x80:
        body = b"\x00" + body
    return _der(0x02, body)


def _is_probable_prime(n, rounds=40):
    if n < 2:
        return False
    for p in (2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37):
        if n % p == 0:
            return n == p
    d, r = n - 1, 0
    while d % 2 == 0:
        d //= 2
        r += 1
    for _ in range(rounds):
        a = 2 + secrets.randbelow(n - 3)
        x = pow(a, d, n)
        if x in (1, n - 1):
            continue
        for _ in range(r - 1):
            x = pow(x, 2, n)
            if x == n - 1:
                break
        else:
            return False
    return True


def _prime(bits):
    while True:
        cand = secrets.randbits(bits) | (1 << (bits - 1)) | 1
        if _is_probable_prime(cand):
            return cand


def rsa_keypair(bits=2048, exponent=65537):
    """A fresh RSA keypair as (n, e, d)."""
    while True:
        p, q = _prime(bits // 2), _prime(bits // 2)
        if p == q:
            continue
        n = p * q
        if n.bit_length() != bits:
            continue
        phi = (p - 1) * (q - 1)
        try:
            d = pow(exponent, -1, phi)
        except ValueError:
            continue
        return n, exponent, d


def public_key_pem(n, e):
    """SubjectPublicKeyInfo PEM (what the server's pem.Decode + ParsePKIXPublicKey read)."""
    alg = _der(0x30, _der(0x06, _OID_RSA) + _der(0x05, b""))
    pkcs1 = _der(0x30, _der_int(n) + _der_int(e))
    spki = _der(0x30, alg + _der(0x03, b"\x00" + pkcs1))
    b64 = base64.b64encode(spki).decode()
    lines = [b64[i:i + 64] for i in range(0, len(b64), 64)]
    return "-----BEGIN PUBLIC KEY-----\n" + "\n".join(lines) + "\n-----END PUBLIC KEY-----\n"


def _sha256_digest_info(payload):
    oid = _der(0x30, _der(0x06, _OID_SHA256) + _der(0x05, b""))
    return _der(0x30, oid + _der(0x04, hashlib.sha256(payload).digest()))


def sign_pkcs1v15_sha256(n, d, payload):
    """RSASSA-PKCS1-v1_5 over the SHA-256 digest of payload."""
    k = (n.bit_length() + 7) // 8
    em = _sha256_digest_info(payload)
    if int.from_bytes(em, "big") >= n:
        raise ValueError("digest info too large for key")
    return pow(int.from_bytes(em, "big"), d, n).to_bytes(k, "big")


def build_identity_message(node, handle="external", name="External E2E", bits=2048):
    """A signed identity DataMessage for a guid this store has never seen.

    Mirrors the server's WriteDataMessage: message is base64 of the
    SignedData JSON (guid, timestamp, messageType, value), the guid is the
    SHA-256 of the PEM the server re-derives it from, and the signature is
    PKCS#1 v1.5 over that same JSON. With node set the receiving server
    (configured with no federation domain) refuses the contact.
    """
    n, e, d = rsa_keypair(bits)
    pem = public_key_pem(n, e)
    guid = hashlib.sha256(pem.encode()).hexdigest()
    identity = {"revision": 1, "handle": handle, "name": name, "description": "",
                "location": "", "image": "", "version": "klatch-e2e", "node": node, "seal": ""}
    signed = {"guid": guid, "timestamp": int(time.time()), "messageType": "identity",
              "value": json.dumps(identity)}
    payload = json.dumps(signed).encode()
    return {
        "message": base64.b64encode(payload).decode(),
        "keyType": "RSA2048",
        "publicKey": base64.b64encode(pem.encode()).decode(),
        "signature": base64.b64encode(sign_pkcs1v15_sha256(n, d, payload)).decode(),
        "signatureType": "PKCS1v15",
    }


def load_provision():
    """The existing examples/klatch admin client (single source of the admin API)."""
    import importlib.util
    spec = importlib.util.spec_from_file_location(
        "klatch_provision", REPO / "examples" / "klatch" / "provision.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def summarize(steps):
    statuses = [steps[name]["status"] for name in STEPS]
    if "fail" in statuses:
        return "FAIL"
    if "blocked" in statuses or "pending" in statuses:
        return "BLOCKED"
    return "PASS"


def leaked(text, secret_values):
    """Names (never values) of secrets appearing in a transcript."""
    return [name for name, value in secret_values.items() if value and value in (text or "")]


def _parse_json(raw):
    text = (raw or "").strip()
    if not text:
        return None
    try:
        return json.loads(text)
    except ValueError:
        return text


class Journey:
    """The roster journey against one sandbox URL (no working tree needed)."""

    def __init__(self, args, url, secret):
        self.args = args
        self.url = url.rstrip("/")
        self.secret = secret
        self.secrets = {"sso_secret": secret}
        self.guid = {}
        self.token = {}
        self.calls = 0
        self.client = None
        self.mod = None

    # ---------- raw HTTP (status codes are assertions here) ----------
    def http(self, method, path, query=None, body=None, headers=None, timeout=30):
        """One request returning (status, parsed-or-raw). Never raises on an HTTP error."""
        url = f"{self.url}{path}"
        if query:
            url += "?" + urllib.parse.urlencode(query)
        data = None
        hdrs = dict(headers or {})
        if body is not None:
            data = json.dumps(body).encode()
            hdrs["Content-Type"] = "application/json"
        req = urllib.request.Request(url, data=data, method=method, headers=hdrs)
        self.calls += 1
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return r.status, _parse_json(r.read().decode())
        except urllib.error.HTTPError as e:
            return e.code, _parse_json(e.read().decode())

    # ---------- sandbox ----------
    def admin_status(self):
        with urllib.request.urlopen(self.url + "/admin/status", timeout=10) as r:
            return json.loads(r.read().decode() or "null")

    def setup(self):
        """Fresh-store gate, endpoint probe, admin bootstrap, empty-domain config."""
        status = self.admin_status()
        if status is not True:
            # a claimed store is not a disposable sandbox; never provision into it
            raise Blocked(f"{STORE_BLOCKED} (admin/status={status!r})")
        probe, _ = self.http("PUT", "/admin/sso/roster", {"token": "endpoint-probe"},
                             {"owner": "probe@example.com", "emails": []})
        reason = endpoint_present(probe)
        if reason:
            raise Blocked(reason)
        mod = load_provision()
        client = mod.Client(self.url, resolve=lambda ref: ref)
        admin = secrets.token_urlsafe(18)
        self.secrets["admin_password"] = admin
        try:
            client.setup_admin(admin)
            session = client.admin_session(admin)
            # empty domain: same-node contacts only, external contacts refused
            client.set_config(session, "")
        except SystemExit as e:  # provision.py reports HTTP failures via SystemExit
            raise RuntimeError(f"sandbox setup: {e}")
        self.secrets["admin_session"] = session
        self.client = client
        self.mod = mod
        return session

    def push_roster(self, owner, emails):
        """The same PUT /admin/sso/roster `bashy klatch sync` makes (Client.set_roster)."""
        try:
            return self.client.set_roster(self.secrets["admin_session"],
                                          {"owner": owner, "emails": list(emails)})
        except SystemExit as e:
            raise RuntimeError(f"roster push: {e}")

    # ---------- single sign-on ----------
    def sso_post(self, email, headers=None):
        """POST /account/sso; returns (status, one-time code or error body)."""
        hdrs = headers if headers is not None else sso_headers(self.secret, email)
        status, body = self.http("POST", "/account/sso", None, None, hdrs)
        names = leaked(str(body), self.secrets)
        if names:
            raise Check(f"secret leaked in /account/sso answer: {names}")
        return status, body

    def sso_sign_in(self, email):
        """The full outpost sign-in: signed identity -> one-time code -> app token."""
        status, code = self.sso_post(email)
        if status != 200 or not isinstance(code, str) or not code:
            raise Check(f"sso sign-in {email}: HTTP {status} {code!r}")
        status, login = self.http("PUT", "/account/access",
                                  {"token": code, "appName": SSO_APP, "platform": "e2e"})
        if status != 200 or not isinstance(login, dict) or not login.get("appToken"):
            raise Check(f"sso code redeem {email}: HTTP {status} {login!r}")
        self.guid[email] = login.get("guid")
        self.token[email] = login["appToken"]
        self.secrets[f"{email}_token"] = login["appToken"]
        return login

    # ---------- owner data ----------
    def channels(self, email):
        try:
            return self.client.channels(self.token[email])
        except SystemExit as e:
            raise RuntimeError(f"channels {email}: {e}")

    def cards(self, email):
        try:
            return self.client.cards(self.token[email])
        except SystemExit as e:
            raise RuntimeError(f"cards {email}: {e}")

    def card_for(self, email, guid):
        for entry in self.cards(email):
            if self.mod.card_guid(entry) == guid:
                return entry
        return None

    def add_card(self, email, message):
        """POST /contact/cards with a signed identity; the status is the assertion."""
        return self.http("POST", "/contact/cards", {"agent": self.token[email]}, message)


# ---------- steps ----------
def step_setup(sb, ctx):
    del ctx  # the emails live in run_journey's context; setup only opens the sandbox
    session = sb.setup()
    if not isinstance(session, str) or not session:
        raise Check("admin session not established")
    return {"federation_domain": "", "endpoint": "present"}, \
        ["sandbox_store_fresh", "roster_endpoint_present", "admin_configured"]


def step_push(sb, ctx):
    first = sb.push_roster(ctx["owner"], [ctx["member"]])
    assert_counts("first roster push", first, members=2, created=2, enabled=0, disabled=0)
    again = sb.push_roster(ctx["owner"], [ctx["member"]])
    assert_counts("repeat roster push", again, members=2, created=0, enabled=0, disabled=0)
    return {"first": roster_counts(first), "repeat": roster_counts(again)}, \
        ["first_push_counts", "repeat_push_noop"]


def step_signin(sb, ctx):
    checks = []
    owner, member = ctx["owner"], ctx["member"]
    a = sb.sso_sign_in(owner)
    b = sb.sso_sign_in(member)
    if not a.get("guid") or not b.get("guid") or a["guid"] == b["guid"]:
        raise Check("roster owner and member share a guid or have none")
    checks += ["owner_signin", "member_signin", "guids_distinct"]
    status, _ = sb.sso_post(ctx["stranger"], sso_headers(sb.secret, ctx["stranger"]))
    if sign_in_answer(status) != "refused":
        raise Check(f"unlisted email sign-in answered {status}, not refused")
    checks.append("unlisted_refused")
    status, _ = sb.sso_post(owner, {"Remote-User": owner})
    if status != 401:
        raise Check(f"unsigned sign-in answered {status}, not 401")
    checks.append("unsigned_refused")
    return {"stranger": ctx["stranger"]}, checks


def step_team(sb, ctx):
    owner, member = ctx["owner"], ctx["member"]
    checks = []
    team = [c for c in sb.channels(owner) if sb.mod.channel_subject(c) == TEAM_SUBJECT]
    if not team:
        raise Check(f"roster owner has no {TEAM_SUBJECT} channel of its own")
    if any(sb.mod.channel_subject(c) == TEAM_SUBJECT for c in sb.channels(member)):
        raise Check(f"member owns a {TEAM_SUBJECT} channel; it belongs to the roster owner")
    checks += ["team_owned_by_roster_owner", "member_not_team_owner"]
    for who, other in ((owner, member), (member, owner)):
        card = sb.card_for(who, sb.guid[other])
        if not card or sb.mod.card_status(card) != "connected":
            raise Check(f"roster did not mesh {who} with {other}")
    checks.append("members_meshed")
    return {"teamChannelId": team[0].get("id")}, checks


def step_remove(sb, ctx):
    owner, member = ctx["owner"], ctx["member"]
    checks = []
    gone = sb.push_roster(owner, [])
    assert_counts("member removal", gone, members=1, created=0, enabled=0, disabled=1)
    checks.append("removal_counts")
    if sb.card_for(owner, sb.guid[member]) is not None:
        raise Check("removed member still in the owner's contacts")
    checks.append("removed_contacts_dropped")
    status, _ = sb.sso_post(member)
    if sign_in_answer(status) != "refused":
        raise Check(f"removed member sign-in answered {status}, not refused")
    checks.append("removed_signin_refused")
    back = sb.push_roster(owner, [member])
    # enabled counts only accounts that existed disabled: the disable itself
    assert_counts("member restore", back, members=2, created=0, enabled=1, disabled=0)
    checks.append("disabled_proven_by_reenable")
    sb.sso_sign_in(member)
    checks.append("restored_signin_works")
    return {"removal": roster_counts(gone), "restore": roster_counts(back)}, checks


def step_federation(sb, ctx):
    owner = ctx["owner"]
    checks = []
    external = build_identity_message(node="https://elsewhere.example.test")
    status, _ = sb.add_card(owner, external)
    if contact_answer(status) != "refused":
        raise Check(f"external-node contact answered {status}, not refused")
    checks.append("external_node_refused")
    unknown = build_identity_message(node="")
    status, _ = sb.add_card(owner, unknown)
    if contact_answer(status) != "refused":
        raise Check(f"unknown-guid contact answered {status}, not refused")
    checks.append("unknown_guid_refused")
    return {"node": "https://elsewhere.example.test"}, checks


STEP_FUNCS = {
    "setup": step_setup, "push": step_push, "signin": step_signin, "team": step_team,
    "remove": step_remove, "federation": step_federation,
}


def run_journey(args, url, secret, step_funcs=None, journey_factory=Journey):
    """Run every roster step against the sandbox at url.

    Returns {"steps", "checks_passed", "skipped_checks", "http_calls",
    "accounts", "journey_teardown"}. Never raises: a failure is a failed step.
    """
    step_funcs = dict(STEP_FUNCS, **(step_funcs or {}))
    stamp = f"{time.strftime('%Y%m%d%H%M%S')}{os.getpid()}"
    ctx = {"owner": f"owner-{stamp}@e2e.example.test",
           "member": f"member-{stamp}@e2e.example.test",
           "stranger": f"stranger-{stamp}@e2e.example.test"}
    steps = {name: {"status": "pending"} for name in STEPS}
    jr = None
    try:
        jr = journey_factory(args, url, secret)
    except Exception as e:  # noqa: BLE001 - no journey, no steps
        steps = {name: {"status": "fail", "reason": f"journey setup: {e}"} for name in STEPS}
    if jr is not None:
        stop_reason = None
        for name in STEPS:
            if stop_reason:
                steps[name] = {"status": "blocked", "reason": stop_reason}
                continue
            t0 = time.time()
            try:
                detail, checks = step_funcs[name](jr, ctx)
                steps[name] = {"status": "pass", "checks": [c for c in checks if c],
                               "detail": detail}
            except Blocked as e:
                steps[name] = {"status": "blocked", "reason": str(e)}
                stop_reason = f"skipped: {name} blocked"
            except Exception as e:  # noqa: BLE001 - any failure is a failed leg
                steps[name] = {"status": "fail", "reason": f"{type(e).__name__}: {e}"}
                stop_reason = f"skipped: {name} failed"
            steps[name]["seconds"] = round(time.time() - t0, 2)
    teardown = {"status": "done", "detail": "no roster journey tree"} \
        if jr is not None else {"status": "skipped", "detail": "no journey ran"}
    return {
        "steps": steps,
        "checks_passed": sum(len(s.get("checks", [])) for s in steps.values()),
        "skipped_checks": [],
        "http_calls": getattr(jr, "calls", 0),
        "accounts": [ctx["owner"], ctx["member"]],
        "journey_teardown": teardown,
    }


def resolve_secret(args):
    """(secret, source) from --sso-secret, KLATCH_E2E_SSO_SECRET or a fresh value."""
    given = getattr(args, "sso_secret", None)
    if given:
        return given, "cli"
    env = os.environ.get(SECRET_ENV)
    if env:
        return env, "env"
    return secrets.token_urlsafe(24), "generated"


def candidate(args):
    """Provenance for the evidence record; never the secret itself."""
    del args
    return {"sso_prefix": SSO_PREFIX, "team_subject": TEAM_SUBJECT}


def add_arguments(ap):
    """The roster options run.py exposes for --suite roster."""
    g = ap.add_argument_group("roster suite")
    g.add_argument("--sso-secret", default=None,
                   help="KLATCH_SSO_SECRET the container runs with (also "
                        f"{SECRET_ENV}; generated fresh when unset)")
    return ap
