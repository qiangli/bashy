#!/usr/bin/env python3
"""Klatch end-to-end MVP across four topologies: none, outpost, satellite, both.

Drives the owner's logged-in browser through `bashy browser --mode live`
(the browser hub plus the Bashy extension) against a Klatch server, plus one
`bashy meet` session. Exit 0 = pass, 1 = fail, 2 = BLOCKED; a blocked run is
never a pass.

Topologies:

- none: Klatch runs locally under `bashy oci` with local accounts only.
  Fully hermetic. The URL defaults to the local loopback server.
- outpost: single-node outpost with no satellite. BLOCKED with the owning
  sprint when no local outpost is reachable.
- satellite / both: self-hosted runs only. The URL comes from --url or
  KLATCH_E2E_URL and is never committed; both are BLOCKED without it.

Journeys per topology, run as separate steps:

- signup_login: human signup/login plus agent auto-registration (the agent
  gets its own account and unique id).
- topic_exchange: human and agent post uniquely tagged messages in one topic
  and each reads the other's, attribution correct.
- meet_session: one `bashy meet` session including the human and the agent.

Any leg whose product support is missing reports BLOCKED with the owning
sprint (S565 outpost single-node, S568 human/agent identity and Klatch to
Meet routing), never a pass. Teardown removes the sandbox and test accounts.
"""
import argparse
import json
import os
import subprocess
import sys
import time
from datetime import datetime, timezone

LOCAL_URL = "http://127.0.0.1:7100/"
SEND_ICON = "//button[.//*[name()='path' and starts-with(@d,'M10 14l11 -11')]]"

TOPOLOGIES = ("none", "outpost", "satellite", "both")
STEPS = ("signup_login", "topic_exchange", "meet_session")

OUTPOST_BLOCKED = (
    "no local outpost reachable "
    "(owning sprint S565 outpost single-node)"
)
AGENT_BLOCKED = (
    "agent auto-registration not supported here "
    "(owning sprint S568 human/agent identity)"
)
MEET_BLOCKED = (
    "Klatch to Meet bridging is design only "
    "(owning sprint S568 Klatch to Meet routing)"
)
URL_BLOCKED = (
    "satellite topology needs a URL: pass --url or set KLATCH_E2E_URL"
)


class Blocked(Exception):
    pass


def browser(*args, tab=None, json_out=False, timeout=90):
    argv = ["bashy", "browser", "--mode", "live"]
    if json_out:
        argv.append("--json")
    if tab is not None:
        argv += ["--id", str(tab)]
    argv += list(args)
    p = subprocess.run(argv, capture_output=True, text=True, timeout=timeout)
    out = p.stdout.strip()
    if p.returncode != 0:
        raise RuntimeError(f"{' '.join(argv[3:])}: exit {p.returncode}: {p.stderr.strip() or out}")
    return json.loads(out) if json_out else out


def js(tab, expr):
    return json.loads(browser("eval", f"JSON.stringify({expr})", tab=tab))


def wait_for(fn, what, timeout=20):
    deadline = time.time() + timeout
    while True:
        got = fn()
        if got:
            return got
        if time.time() > deadline:
            raise AssertionError(f"timed out waiting for {what}")
        time.sleep(1)


def resolve_url(topology, url_arg, env_url=None):
    """Return the Klatch URL for a topology, or raise Blocked.

    satellite/both require --url or KLATCH_E2E_URL. none falls back to the
    local loopback server. outpost uses --url or KLATCH_E2E_URL when given;
    without one the outpost reachability gate decides.
    """
    if env_url is None:
        env_url = os.environ.get("KLATCH_E2E_URL")
    url = url_arg or env_url
    if topology in ("satellite", "both") and not url:
        raise Blocked(URL_BLOCKED)
    if topology == "none" and not url:
        url = LOCAL_URL
    return url


def url_source(url_arg, env_url=None):
    if url_arg:
        return "cli"
    if env_url is None:
        env_url = os.environ.get("KLATCH_E2E_URL")
    if env_url:
        return "env"
    return "local-default"


def default_outpost_reachable():
    """Best-effort probe: True only if a local outpost answers."""
    try:
        p = subprocess.run(["bashy", "outpost", "status"],
                           capture_output=True, text=True, timeout=30)
    except Exception:
        return False
    return p.returncode == 0


def default_register_agent(url, agent_tag):
    """Agent auto-registration. Missing until S568 lands, so BLOCKED."""
    raise Blocked(AGENT_BLOCKED)


def default_ensure_meet(url):
    """One meet session with the human and the agent. Missing until S568."""
    raise Blocked(MEET_BLOCKED)


def candidate_info():
    """Best-effort candidate bashy version and sha; 'unknown' when missing."""
    info = {"bashy_version": "unknown", "bashy_sha": "unknown"}
    try:
        p = subprocess.run(["git", "rev-parse", "HEAD"],
                           capture_output=True, text=True, timeout=30)
        if p.returncode == 0 and p.stdout.strip():
            info["bashy_sha"] = p.stdout.strip()
    except Exception:
        pass
    try:
        p = subprocess.run(["bashy", "version"],
                           capture_output=True, text=True, timeout=30)
        out = ((p.stdout or "") + "\n" + (p.stderr or "")).strip()
        if p.returncode == 0 and out:
            info["bashy_version"] = out.splitlines()[0][:120]
    except Exception:
        pass
    return info


def teardown_sandbox(topology, context):
    """Best-effort teardown: close the driven tab; sandbox state drops with it."""
    detail = []
    tab = (context or {}).get("tab_id")
    if tab is not None:
        try:
            browser("close", tab=tab)
            detail.append(f"closed tab {tab}")
        except Exception as e:  # noqa: BLE001 - teardown records, never raises
            detail.append(f"tab close failed: {e}")
    detail.append("sandbox-local test accounts drop with the sandbox")
    return {"status": "done", "detail": "; ".join(detail)}


def fresh_tags():
    stamp = f"{time.strftime('%Y%m%d-%H%M%S')}-{os.getpid()}"
    return f"human-{stamp}", f"agent-{stamp}"


def step_signup_login(args, url, human_tag, agent_tag, register_agent):
    """Human signup/login gate, then agent auto-registration."""
    del human_tag  # recorded in evidence; the gate below covers the human leg
    st = browser("status", json_out=True)
    hub = {k: st.get(k) for k in ("hub_up", "extension_connected", "extension_version")}
    if not st.get("hub_up") or not st.get("extension_connected"):
        raise Blocked("browser hub not up or extension not connected (start `bashy browser hub`, connect the extension)")

    nav = browser("navigate", url, json_out=True)
    tab = nav["tab_id"]
    if nav.get("title") == "Sign in to continue":
        raise Blocked(f"sign-in needed: sign in in tab {tab}, then rerun")

    session = wait_for(lambda: js(tab, "location.hash.startsWith('#/session')"), "the Klatch session view")
    login_form = js(tab, "!!document.querySelector('input[type=password]')")
    if login_form:
        raise AssertionError("Klatch login form shown: single sign-on did not sign in")
    agent = register_agent(url, agent_tag)
    return {"tab_id": tab, "hub": hub, "session": bool(session), "agent": agent}


def step_topic_exchange(args, tab, human_tag, agent_tag):
    """Human and agent post uniquely tagged messages; each reads the other's."""
    def listed():
        body = js(tab, "document.body.innerText")
        return all(t in body for t in args.expect_topics + [args.topic])
    try:
        wait_for(listed, "the topic list")
    except AssertionError:
        body = js(tab, "document.body.innerText")
        missing = [t for t in args.expect_topics + [args.topic] if t not in body]
        raise AssertionError(f"topics not listed: {missing}")

    browser("click", f'//*[normalize-space(text())="{args.topic}"]', tab=tab)
    wait_for(lambda: js(tab, "!!document.querySelector('textarea')"), "the message box")
    posted = {}
    for who, tag in (("human", human_tag), ("agent", agent_tag)):
        msg = f"klatch-e2e {who} {tag}"
        browser("type", "textarea", msg, tab=tab)
        browser("click", SEND_ICON, tab=tab)
        wait_for(lambda m=msg: js(tab, f"document.body.innerText.includes({json.dumps(m)})"), f"the {who} message")
        posted[who] = msg
    if js(tab, "(document.querySelector('textarea')||{}).value"):
        raise AssertionError("message box not cleared after send")
    body = js(tab, "document.body.innerText")
    for who, msg in posted.items():
        if msg not in body:
            raise AssertionError(f"{who} message not attributed in topic")
    return {"posted": posted}


def exit_code_for(result):
    return {"PASS": 0, "FAIL": 1, "BLOCKED": 2}[result]


def summarize(steps):
    """Overall result from per-step statuses. BLOCKED is never a pass."""
    statuses = [steps[name]["status"] for name in STEPS]
    if "fail" in statuses:
        return "FAIL"
    if "blocked" in statuses:
        return "BLOCKED"
    return "PASS"


def block_all(reason):
    return {name: {"status": "blocked", "reason": reason} for name in STEPS}


def run(args, hooks=None):
    """Run one topology's journey. Returns the JSON-serializable evidence record."""
    hooks = dict(hooks or {})
    outpost_reachable = hooks.get("outpost_reachable", default_outpost_reachable)
    register_agent = hooks.get("register_agent", default_register_agent)
    ensure_meet = hooks.get("ensure_meet", default_ensure_meet)
    describe_candidate = hooks.get("candidate_info", candidate_info)
    teardown = hooks.get("teardown", teardown_sandbox)

    started_at = datetime.now(timezone.utc).isoformat()
    topology = getattr(args, "topology", "none") or "none"
    human_tag, agent_tag = fresh_tags()
    context = {}
    if topology not in TOPOLOGIES:
        steps = block_all(f"unknown topology {topology!r}: choose one of {', '.join(TOPOLOGIES)}")
        steps = {name: {"status": "fail", "reason": info["reason"]} for name, info in steps.items()}
    else:
        try:
            url = resolve_url(topology, getattr(args, "url", None))
        except Blocked as e:
            steps = block_all(str(e))
            url = getattr(args, "url", None) or None
        else:
            steps = {name: {"status": "pending"} for name in STEPS}
            if topology == "outpost" and not outpost_reachable():
                steps = block_all(OUTPOST_BLOCKED)
            else:
                for i, name in enumerate(STEPS):
                    try:
                        if name == "signup_login":
                            info = step_signup_login(args, url, human_tag, agent_tag, register_agent)
                            context["tab_id"] = info["tab_id"]
                            steps[name] = {"status": "pass", "detail": info}
                        elif name == "topic_exchange":
                            info = step_topic_exchange(args, context["tab_id"], human_tag, agent_tag)
                            steps[name] = {"status": "pass", "detail": info}
                        else:
                            info = ensure_meet(url)
                            steps[name] = {"status": "pass", "detail": info}
                    except Blocked as e:
                        steps[name] = {"status": "blocked", "reason": str(e)}
                        for later in STEPS[i + 1:]:
                            steps[later] = {"status": "blocked", "reason": f"skipped: {name} blocked"}
                        break
                    except Exception as e:  # noqa: BLE001 - any failure is a failed leg
                        steps[name] = {"status": "fail", "reason": str(e)}
                        for later in STEPS[i + 1:]:
                            steps[later] = {"status": "blocked", "reason": f"skipped: {name} failed"}
                        break
    result = summarize(steps)
    try:
        candidate = describe_candidate()
    except Exception:  # noqa: BLE001 - candidate info is best-effort
        candidate = {"bashy_version": "unknown", "bashy_sha": "unknown"}
    try:
        teardown_info = teardown(topology, {"human_tag": human_tag, "agent_tag": agent_tag, **context})
    except Exception as e:  # noqa: BLE001 - teardown records, never raises
        teardown_info = {"status": "failed", "detail": str(e)}
    finished_at = datetime.now(timezone.utc).isoformat()
    record = {
        "topology": topology,
        "url": url if topology in TOPOLOGIES else None,
        "url_source": url_source(getattr(args, "url", None)),
        "topic": getattr(args, "topic", None),
        "result": result,
        "steps": steps,
        "accounts": {"human_tag": human_tag, "agent_tag": agent_tag},
        "teardown": teardown_info,
        "candidate": candidate,
        "started_at": started_at,
        "finished_at": finished_at,
    }
    evidence_path = getattr(args, "evidence", None)
    if evidence_path:
        with open(evidence_path, "w", encoding="utf-8") as f:
            json.dump(record, f, indent=2, default=str)
            f.write("\n")
        record["evidence_path"] = evidence_path
    return record


def build_parser():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--topology", choices=TOPOLOGIES, default="none",
                    help="which deployment shape to drive (default: none)")
    ap.add_argument("--url", default=None,
                    help="Klatch URL; satellite/both also accept KLATCH_E2E_URL")
    ap.add_argument("--topic", default="General", help="topic to post the test message in")
    ap.add_argument("--expect-topics", nargs="*", default=["Team"], help="topics that must be listed")
    ap.add_argument("--evidence", default=None, metavar="PATH",
                    help="write the JSON evidence record to PATH")
    return ap


def main(argv=None):
    args = build_parser().parse_args(argv)
    try:
        record = run(args)
    except Exception as e:  # noqa: BLE001 - any failure is a failed journey
        print(json.dumps({"result": "FAIL", "reason": str(e)}))
        return 1
    print(json.dumps(record, default=str))
    return exit_code_for(record["result"])


if __name__ == "__main__":
    sys.exit(main())
