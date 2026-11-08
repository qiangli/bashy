#!/usr/bin/env python3
"""Klatch end-to-end MVP across four topologies: none, outpost, satellite, both.

Drives the owner's logged-in browser through `bashy browser --mode live`
(the browser hub plus the Bashy extension) against a Klatch server, plus one
`bashy meet` session. Exit 0 = pass, 1 = fail, 2 = BLOCKED; a blocked run is
never a pass.

Topologies:

- none: starts Klatch locally with `bashy oci run` (the Klatch image built
  from builds/klatch, auto-detected in local storage or via --image /
  KLATCH_E2E_URL-independent KLATCH_E2E_IMAGE), waits for the server to
  answer HTTP health on the URL, runs the journeys against it, then removes
  the container. A start that fails after creating the container removes it
  again and still writes the evidence file. When `bashy oci` or the image
  is unavailable on the host the run is BLOCKED with the concrete reason,
  never a pass.
- outpost: single-node outpost with no satellite. The URL comes from --url
  or KLATCH_E2E_URL (BLOCKED without one); the local outpost only counts as
  reachable when `bashy outpost status` succeeds AND `bashy outpost apps
  list` shows an enabled klatch app. Otherwise BLOCKED with the owning
  sprint.
- satellite / both: self-hosted runs only. The URL comes from --url or
  KLATCH_E2E_URL and is never committed; both are BLOCKED without it.

Journeys per topology, run as separate steps:

- signup_login: human signup/login plus agent auto-registration (the agent
  gets its own account and unique id).
- topic_exchange: the human posts from their own browser tab and the agent
  posts under its own identity; each side's message is read back and the
  sender of each message is asserted.
- meet_session: one `bashy meet` session including the human and the agent.

Suites (--suite):

- browser (default): the journeys above.
- participant: the participant CLI journey from participant.py (an imported
  helper; it never starts a server). Topology none only: this runner starts a
  disposable Klatch container with `bashy oci run`, waits for health, runs the
  journey against the container URL (fresh store, two fresh accounts, sealed
  conversation, messages, attachments, watch, the Bash# wrapper), then removes
  the container, which drops the store and the accounts. Missing node or helper
  is BLOCKED before any container starts; any other topology is BLOCKED,
  because the journey claims the store and may only run against a disposable
  server. `Containerfile.server` builds a suitable image from a Klatch tree.

Any leg whose product support is missing reports BLOCKED with the owning
sprint (S565 outpost single-node, S568 human/agent identity and Klatch to
Meet routing), never a pass. Teardown closes the driven tab, removes the
locally started container (none), and drops the sandbox test accounts.
"""
import argparse
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from pathlib import Path
import importlib.util


def _load_participant():
    spec = importlib.util.spec_from_file_location(
        "klatch_e2e_participant", Path(__file__).resolve().with_name("participant.py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


participant = _load_participant()

LOCAL_URL = "http://127.0.0.1:7100/"
SEND_ICON = "//button[.//*[name()='path' and starts-with(@d,'M10 14l11 -11')]]"

TOPOLOGIES = ("none", "outpost", "satellite", "both")
SUITES = ("browser", "participant")
STEPS = ("signup_login", "topic_exchange", "meet_session")

OUTPOST_URL_BLOCKED = (
    "outpost topology needs a URL: pass --url or set KLATCH_E2E_URL "
    "(owning sprint S565 outpost single-node)"
)
OUTPOST_BLOCKED = (
    "no local outpost serving klatch "
    "(owning sprint S565 outpost single-node)"
)
AGENT_BLOCKED = (
    "agent auto-registration not supported here "
    "(owning sprint S568 human/agent identity)"
)
AGENT_POST_BLOCKED = (
    "agent topic post under its own identity not supported here "
    "(owning sprint S568 human/agent identity)"
)
MEET_BLOCKED = (
    "Klatch to Meet bridging is design only "
    "(owning sprint S568 Klatch to Meet routing)"
)
PARTICIPANT_TOPOLOGY_BLOCKED = (
    "participant suite claims the server store, so it only runs against the "
    "disposable container of topology none"
)
URL_BLOCKED = (
    "satellite topology needs a URL: pass --url or set KLATCH_E2E_URL"
)


class Blocked(Exception):
    pass


def _bashy_bin():
    return os.environ.get("BASHY_BIN", "bashy")


def browser(*args, tab=None, json_out=False, timeout=90):
    argv = [_bashy_bin(), "browser", "--mode", "live"]
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

    satellite/both and outpost require --url or KLATCH_E2E_URL. none falls
    back to the local loopback server.
    """
    if env_url is None:
        env_url = os.environ.get("KLATCH_E2E_URL")
    url = url_arg or env_url
    if topology in ("satellite", "both") and not url:
        raise Blocked(URL_BLOCKED)
    if topology == "outpost" and not url:
        raise Blocked(OUTPOST_URL_BLOCKED)
    if topology == "none" and not url:
        url = LOCAL_URL
    return url


def url_source(topology, url_arg, env_url=None):
    if url_arg:
        return "cli"
    if env_url is None:
        env_url = os.environ.get("KLATCH_E2E_URL")
    if env_url:
        return "env"
    if topology == "none":
        return "local-default"
    return "missing"


def default_outpost_reachable():
    """True only if the local outpost is up AND serves an enabled klatch app."""
    try:
        p = subprocess.run([_bashy_bin(), "outpost", "status"],
                           capture_output=True, text=True, timeout=30)
    except Exception:
        return False
    if p.returncode != 0:
        return False
    try:
        q = subprocess.run([_bashy_bin(), "outpost", "apps", "list"],
                           capture_output=True, text=True, timeout=30)
    except Exception:
        return False
    if q.returncode != 0:
        return False
    for line in (q.stdout or "").splitlines():
        toks = line.split()
        if len(toks) >= 3 and toks[0].lower() == "klatch" and toks[2].lower() == "true":
            return True
    return False


def find_klatch_image(images_json):
    """First local image whose name mentions klatch, else None."""
    try:
        entries = json.loads(images_json or "[]")
    except ValueError:
        entries = []
    if isinstance(entries, dict):
        entries = [entries]
    for entry in entries:
        if not isinstance(entry, dict):
            continue
        names = entry.get("Names") or entry.get("names") or []
        repo = entry.get("Repository") or entry.get("repository") or ""
        tag = entry.get("Tag") or entry.get("tag") or "latest"
        if repo and repo not in (None, "<none>"):
            names = list(names) + [f"{repo}:{tag}"]
        for name in names:
            if "klatch" in str(name).lower():
                return str(name)
    m = re.search(r'"([\w./:@-]*klatch[\w./:@-]*)"', images_json or "", re.IGNORECASE)
    return m.group(1) if m else None


def wait_for_health(url, timeout=90):
    deadline = time.time() + timeout
    while True:
        try:
            with urllib.request.urlopen(url, timeout=5):
                return True
        except urllib.error.HTTPError:
            return True  # the server answered; it is up
        except Exception:  # noqa: BLE001 - not up yet; keep polling
            pass
        if time.time() > deadline:
            raise RuntimeError(f"klatch server at {url} not healthy after {timeout}s")
        time.sleep(1)


def _remove_container_best_effort(oci, name):
    """`oci rm -f` that never raises; teardown records, never fails the run."""
    try:
        subprocess.run(oci + ["rm", "-f", name],
                       capture_output=True, text=True, timeout=60)
    except Exception:  # noqa: BLE001 - best-effort container cleanup
        pass


def url_answers(url):
    """True when something already serves url (any HTTP answer counts)."""
    try:
        with urllib.request.urlopen(url, timeout=3):
            return True
    except urllib.error.HTTPError:
        return True
    except Exception:  # noqa: BLE001 - nothing listening
        return False


def default_start_server(topology, url, image, container_port=None):
    """Start Klatch locally with `bashy oci run` for the none topology.

    Returns {"container": name, "image": image}. Anything the host cannot
    provide (engine, image) is Blocked, never a pass. A URL that already
    answers before the container exists belongs to another server (possibly a
    live one), so it is Blocked rather than driven. When the container was
    created but the server never became healthy, the container is removed
    before the error propagates, so a failed start never leaks it.
    container_port is the port the image listens on (default: the URL port).
    """
    if topology != "none":
        return None
    if image is None:
        image = os.environ.get("KLATCH_E2E_IMAGE")
    oci = [_bashy_bin(), "oci"]
    try:
        p = subprocess.run(oci + ["images", "--format", "json"],
                           capture_output=True, text=True, timeout=60)
    except Exception as e:  # noqa: BLE001 - engine probe failed
        raise Blocked(f"bashy oci unavailable: {e}")
    combined = ((p.stdout or "") + "\n" + (p.stderr or "")).strip()
    if p.returncode != 0 or "is not running" in combined:
        lines = combined.splitlines() or ["unknown engine error"]
        informative = next(
            (ln for ln in lines if "not running" in ln or ln.startswith("Error")),
            lines[0],
        )
        raise Blocked(f"bashy oci unavailable: {informative[:200]}")
    wanted = image or find_klatch_image(p.stdout or "")
    if not wanted:
        raise Blocked("no klatch image in local oci storage "
                      "(builds/klatch); pass --image or set KLATCH_E2E_IMAGE")
    parts = urllib.parse.urlsplit(url)
    port = parts.port or (443 if parts.scheme == "https" else 80)
    if url_answers(url):
        raise Blocked(f"{url} already answers before the sandbox container exists; "
                      "another server owns that port: pass --url with a free port "
                      "(and --container-port for the image's port)")
    name = f"klatch-e2e-{os.getpid()}-{time.strftime('%Y%m%d-%H%M%S')}"
    try:
        r = subprocess.run(
            oci + ["run", "-d", "--name", name,
                   "-p", f"127.0.0.1:{port}:{container_port or port}", wanted],
            capture_output=True, text=True, timeout=120)
    except Exception as e:  # noqa: BLE001 - engine answered before; this is a failure
        _remove_container_best_effort(oci, name)
        raise RuntimeError(f"bashy oci run failed: {e}")
    if r.returncode != 0:
        _remove_container_best_effort(oci, name)
        raise RuntimeError(f"bashy oci run failed: {(r.stderr or r.stdout).strip()[:300]}")
    try:
        wait_for_health(url)
    except Exception:
        _remove_container_best_effort(oci, name)
        raise
    return {"container": name, "image": wanted, "port": f"127.0.0.1:{port}->{container_port or port}"}


def default_stop_server(server):
    """Remove the locally started container. Best-effort; never raises."""
    if not server:
        return {"status": "skipped", "detail": "no local container started"}
    name = server.get("container", "")
    try:
        p = subprocess.run([_bashy_bin(), "oci", "rm", "-f", name],
                           capture_output=True, text=True, timeout=60)
    except Exception as e:  # noqa: BLE001 - teardown records, never raises
        return {"status": "failed", "detail": f"remove container {name}: {e}"}
    if p.returncode != 0:
        return {"status": "failed",
                "detail": f"remove container {name}: {(p.stderr or p.stdout).strip()[:200]}"}
    return {"status": "done", "detail": f"removed container {name}"}


def default_register_agent(url, agent_tag):
    """Agent auto-registration. Missing until S568 lands, so BLOCKED."""
    raise Blocked(AGENT_BLOCKED)


def default_agent_post(url, agent, agent_tag):
    """Agent topic post under the agent's own identity. Missing until S568."""
    raise Blocked(AGENT_POST_BLOCKED)


def default_read_sender(tab, message):
    """Sender label displayed next to a message in the topic view.

    Reading another identity's attribution needs the S568 identity work, so
    this default is BLOCKED, never a silent skip: no sender is ever assumed.
    """
    raise Blocked(
        "message sender attribution is not readable here "
        "(owning sprint S568 human/agent identity)"
    )


def default_ensure_meet(url):
    """One meet session with the human and the agent. Missing until S568."""
    raise Blocked(MEET_BLOCKED)


def parse_bashy_version(text):
    """Split `bashy --version` output into (version_line, sha)."""
    lines = (text or "").strip().splitlines()
    first = lines[0].strip() if lines else ""
    m = re.search(r"\(([0-9a-f]{7,40})\)\s*$", first)
    return (first or "unknown", m.group(1) if m else "unknown")


def candidate_info():
    """Version and build id of the bashy binary under test; 'unknown' when missing."""
    info = {"bashy_version": "unknown", "bashy_sha": "unknown"}
    try:
        p = subprocess.run([_bashy_bin(), "--version"],
                           capture_output=True, text=True, timeout=30)
    except Exception:
        return info
    out = ((p.stdout or "") + "\n" + (p.stderr or "")).strip()
    if p.returncode == 0 and out:
        version, sha = parse_bashy_version(out)
        info["bashy_version"] = version
        info["bashy_sha"] = sha
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
    if not isinstance(agent, dict) or not agent.get("agent_id"):
        raise AssertionError(
            "agent registration returned no agent_id: cannot assert message "
            "attribution (owning sprint S568 human/agent identity)"
        )
    return {"tab_id": tab, "hub": hub, "session": bool(session), "agent": agent}


def step_topic_exchange(args, tab, url, human_tag, agent_tag, agent, agent_post,
                        read_sender):
    """Human posts from their own tab; agent posts under its own identity.

    Each side reads the other's message back and the sender of each message
    is asserted: the agent receipt must carry the registered agent_id, and
    the human message must carry a sender distinct from the agent. The agent
    message is never typed from the human tab. A missing id is never
    skipped: it fails the step.
    """
    agent_id = agent.get("agent_id") if isinstance(agent, dict) else None
    if not agent_id:
        raise AssertionError(
            "agent record has no agent_id: cannot assert message "
            "attribution (owning sprint S568 human/agent identity)"
        )
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
    human_msg = f"klatch-e2e human {human_tag}"
    browser("type", "textarea", human_msg, tab=tab)
    browser("click", SEND_ICON, tab=tab)
    wait_for(lambda m=human_msg: js(tab, f"document.body.innerText.includes({json.dumps(m)})"),
             "the human message")
    if js(tab, "(document.querySelector('textarea')||{}).value"):
        raise AssertionError("message box not cleared after send")

    receipt = agent_post(url, agent, agent_tag)
    agent_msg = (receipt or {}).get("message", "")
    sender = (receipt or {}).get("sender")
    if not sender:
        raise AssertionError("agent post returned no sender attribution")
    if sender != agent_id:
        raise AssertionError(f"agent message attribution mismatch: sender {sender!r} != agent {agent_id!r}")
    if not agent_msg:
        raise AssertionError("agent post returned no message")
    wait_for(lambda m=agent_msg: js(tab, f"document.body.innerText.includes({json.dumps(m)})"),
             "the agent message")

    human_sender = read_sender(tab, human_msg)
    if not human_sender:
        raise AssertionError(f"human message has no sender attribution in topic {args.topic!r}")
    if human_sender == agent_id:
        raise AssertionError(f"human message misattributed to the agent identity {agent_id!r}")

    body = js(tab, "document.body.innerText")
    for who, msg in (("human", human_msg), ("agent", agent_msg)):
        if msg not in body:
            raise AssertionError(f"{who} message not attributed in topic")
    return {"posted": {"human": human_msg, "agent": agent_msg},
            "agent_sender": sender, "human_sender": human_sender}


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


def _container_port_kw(args):
    port = getattr(args, "container_port", None)
    return {"container_port": port} if port else {}


def _write_evidence(args, record):
    evidence_path = getattr(args, "evidence", None)
    if evidence_path:
        with open(evidence_path, "w", encoding="utf-8") as f:
            json.dump(record, f, indent=2, default=str)
            f.write("\n")
        record["evidence_path"] = evidence_path
    return record


def run_participant(args, hooks=None):
    """The participant suite inside this runner's container lifecycle.

    Order: preflight (BLOCKED, no container) -> start_server (bashy oci run +
    health) -> journey(args, url) -> stop_server (always, once started).
    """
    hooks = dict(hooks or {})
    start_server = hooks.get("start_server", default_start_server)
    stop_server = hooks.get("stop_server", default_stop_server)
    describe_candidate = hooks.get("candidate_info", candidate_info)
    preflight = hooks.get("participant_preflight", participant.preflight)
    journey = hooks.get("participant_journey", participant.run_journey)
    names = participant.STEPS

    started_at = datetime.now(timezone.utc).isoformat()
    topology = getattr(args, "topology", "none") or "none"
    url = getattr(args, "url", None) or None
    server = None
    server_stop = {"status": "skipped", "detail": "no local container started"}
    outcome = {}

    def every(status, reason):
        return {name: {"status": status, "reason": reason} for name in names}

    if topology != "none":
        steps = every("blocked", PARTICIPANT_TOPOLOGY_BLOCKED)
    else:
        url = resolve_url(topology, url)
        reason = preflight(args)
        if reason:
            steps = every("blocked", reason)
        else:
            try:
                server = start_server(topology, url, getattr(args, "image", None),
                                      **_container_port_kw(args))
            except Blocked as e:
                steps = every("blocked", str(e))
            except Exception as e:  # noqa: BLE001 - start crashed: fail, still write evidence
                steps = every("fail", f"klatch container start failed: {e}")
            else:
                try:
                    outcome = journey(args, url) or {}
                    steps = outcome.get("steps") or every("fail", "journey returned no steps")
                except Exception as e:  # noqa: BLE001 - the journey never raises; be safe anyway
                    steps = every("fail", f"participant journey crashed: {e}")
                finally:
                    try:
                        server_stop = stop_server(server)
                    except Exception as e:  # noqa: BLE001 - teardown records, never raises
                        server_stop = {"status": "failed", "detail": str(e)}
    result = participant.summarize(steps)
    try:
        candidate = describe_candidate()
    except Exception:  # noqa: BLE001 - candidate info is best-effort
        candidate = {"bashy_version": "unknown", "bashy_sha": "unknown"}
    try:
        candidate.update(participant.candidate(args))
    except Exception:  # noqa: BLE001 - provenance is best-effort
        pass
    journey_teardown = outcome.get("journey_teardown",
                                   {"status": "skipped", "detail": "no journey ran"})
    torn_down = (server_stop.get("status") in ("done", "skipped")
                 and journey_teardown.get("status") in ("done", "skipped"))
    record = {
        "suite": "participant",
        "topology": topology,
        "url": url,
        "result": result,
        "steps": steps,
        "checks_passed": outcome.get("checks_passed", 0),
        "skipped_checks": outcome.get("skipped_checks", []),
        "cli_calls": outcome.get("cli_calls", 0),
        "accounts": {"handles": outcome.get("accounts", []),
                     "lifetime": "dropped with the sandbox container"},
        "server": server or {"status": "not-started"},
        "server_stop": server_stop,
        "teardown": {"status": "done" if torn_down else "failed",
                     "detail": f"{journey_teardown.get('detail', '')}; {server_stop.get('detail', '')}"},
        "candidate": candidate,
        "started_at": started_at,
        "finished_at": datetime.now(timezone.utc).isoformat(),
    }
    return _write_evidence(args, record)


def run(args, hooks=None):
    """Run one topology's journey. Returns the JSON-serializable evidence record."""
    if getattr(args, "suite", "browser") == "participant":
        return run_participant(args, hooks)
    hooks = dict(hooks or {})
    outpost_reachable = hooks.get("outpost_reachable", default_outpost_reachable)
    start_server = hooks.get("start_server", default_start_server)
    stop_server = hooks.get("stop_server", default_stop_server)
    register_agent = hooks.get("register_agent", default_register_agent)
    agent_post = hooks.get("agent_post", default_agent_post)
    read_sender = hooks.get("read_sender", default_read_sender)
    ensure_meet = hooks.get("ensure_meet", default_ensure_meet)
    describe_candidate = hooks.get("candidate_info", candidate_info)
    teardown = hooks.get("teardown", teardown_sandbox)

    started_at = datetime.now(timezone.utc).isoformat()
    topology = getattr(args, "topology", "none") or "none"
    image = getattr(args, "image", None)
    human_tag, agent_tag = fresh_tags()
    context = {}
    server = None
    server_stop = {"status": "skipped", "detail": "no local container started"}
    if topology not in TOPOLOGIES:
        steps = block_all(f"unknown topology {topology!r}: choose one of {', '.join(TOPOLOGIES)}")
        steps = {name: {"status": "fail", "reason": info["reason"]} for name, info in steps.items()}
        url = getattr(args, "url", None) or None
    else:
        try:
            url = resolve_url(topology, getattr(args, "url", None))
        except Blocked as e:
            steps = block_all(str(e))
            url = getattr(args, "url", None) or None
        else:
            steps = {name: {"status": "pending"} for name in STEPS}
            try:
                if topology == "none":
                    try:
                        server = start_server(topology, url, image, **_container_port_kw(args))
                    except Blocked as e:
                        steps = block_all(str(e))
                    except Exception as e:  # noqa: BLE001 - start crashed: fail, still write evidence
                        steps = {name: {"status": "fail",
                                                 "reason": f"klatch container start failed: {e}"}
                                 for name in STEPS}
                if all(steps[name]["status"] == "pending" for name in STEPS):
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
                                    agent = steps["signup_login"]["detail"]["agent"]
                                    info = step_topic_exchange(args, context["tab_id"], url,
                                                             human_tag, agent_tag, agent, agent_post,
                                                             read_sender)
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
            finally:
                if server is not None:
                    try:
                        server_stop = stop_server(server)
                    except Exception as e:  # noqa: BLE001 - teardown records, never raises
                        server_stop = {"status": "failed", "detail": str(e)}
    result = summarize(steps)
    try:
        candidate = describe_candidate()
    except Exception:  # noqa: BLE001 - candidate info is best-effort
        candidate = {"bashy_version": "unknown", "bashy_sha": "unknown"}
    try:
        teardown_info = teardown(topology, {"human_tag": human_tag, "agent_tag": agent_tag, **context})
    except Exception as e:  # noqa: BLE001 - teardown records, never raises
        teardown_info = {"status": "failed", "detail": str(e)}
    if isinstance(teardown_info, dict):
        teardown_info = dict(teardown_info)
        teardown_info["detail"] = f"{teardown_info.get('detail', '')}; {server_stop.get('detail', '')}".strip("; ")
    finished_at = datetime.now(timezone.utc).isoformat()
    record = {
        "topology": topology,
        "url": url if topology in TOPOLOGIES else None,
        "url_source": url_source(topology, getattr(args, "url", None)),
        "topic": getattr(args, "topic", None),
        "result": result,
        "steps": steps,
        "accounts": {"human_tag": human_tag, "agent_tag": agent_tag},
        "server": server or {"status": "not-started"},
        "server_stop": server_stop,
        "teardown": teardown_info,
        "candidate": candidate,
        "started_at": started_at,
        "finished_at": finished_at,
    }
    return _write_evidence(args, record)


def build_parser():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--topology", choices=TOPOLOGIES, default="none",
                    help="which deployment shape to drive (default: none)")
    ap.add_argument("--url", default=None,
                    help="Klatch URL; satellite/both/outpost also accept KLATCH_E2E_URL")
    ap.add_argument("--image", default=None,
                    help="Klatch container image for --topology none; also accepts KLATCH_E2E_IMAGE")
    ap.add_argument("--topic", default="General", help="topic to post the test message in")
    ap.add_argument("--expect-topics", nargs="*", default=["Team"], help="topics that must be listed")
    ap.add_argument("--evidence", default=None, metavar="PATH",
                    help="write the JSON evidence record to PATH")
    ap.add_argument("--container-port", type=int, default=None,
                    help="port the --topology none image listens on (default: the URL port; "
                         "Containerfile.server listens on 7100)")
    ap.add_argument("--suite", choices=SUITES, default="browser",
                    help="browser journeys (default) or the participant CLI journey")
    participant.add_arguments(ap)
    return ap


def main(argv=None):
    args = build_parser().parse_args(argv)
    if args.suite == "participant":
        participant.resolve_args(args)
    try:
        record = run(args)
    except Exception as e:  # noqa: BLE001 - any failure is a failed journey
        print(json.dumps({"result": "FAIL", "reason": str(e)}))
        return 1
    print(json.dumps(record, default=str))
    return exit_code_for(record["result"])


if __name__ == "__main__":
    sys.exit(main())
