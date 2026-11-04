#!/usr/bin/env python3
"""Klatch invite/chat happy path through the satellite, in the owner's real browser.

Drives the owner's logged-in Chrome through `bashy browser --mode live` (the
browser hub plus the Bashy MV3 extension). Exit 0 = pass, 1 = fail,
2 = BLOCKED (hub or extension missing, or cloudbox sign-in needed); a blocked
run is never a pass. The run always opens its own new tab and posts only in
--topic, so use a topic no one else reads (default: General).

Journey (Sprint 563 J6, docs/plan-s563-master-execution.md):
  1. hub up and extension connected
  2. open the Klatch URL through the satellite in a new tab
  3. cloudbox sign-in gate passed (else BLOCKED: sign in in the opened tab)
  4. single sign-on lands in the Klatch session, no Klatch login form
  5. the expected topics are listed (default: Team)
  6. post a uniquely tagged message in --topic and read it back
"""
import argparse
import json
import subprocess
import sys
import time

DEFAULT_URL = "https://165-227-49-91.sslip.io/matrix/h/elephant/app/klatch/"
SEND_ICON = "//button[.//*[name()='path' and starts-with(@d,'M10 14l11 -11')]]"


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


def run(args):
    evidence = {"url": args.url, "topic": args.topic, "expect_topics": args.expect_topics}

    st = browser("status", json_out=True)
    evidence["hub"] = {k: st.get(k) for k in ("hub_up", "extension_connected", "extension_version")}
    if not st.get("hub_up") or not st.get("extension_connected"):
        raise Blocked("browser hub not up or extension not connected (start `bashy browser hub`, connect the extension)")

    nav = browser("navigate", args.url, json_out=True)
    tab = nav["tab_id"]
    evidence["tab_id"] = tab
    if nav.get("title") == "Sign in to continue":
        raise Blocked(f"cloudbox sign-in needed: sign in in tab {tab}, then rerun")

    session = wait_for(lambda: js(tab, "location.hash.startsWith('#/session')"), "the Klatch session view")
    evidence["session"] = bool(session)
    login_form = js(tab, "!!document.querySelector('input[type=password]')")
    if login_form:
        raise AssertionError("Klatch login form shown: single sign-on did not sign in")

    # topics load after the session view appears
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
    msg = f"klatch-e2e {time.strftime('%Y-%m-%d %H:%M:%S')}"
    browser("type", "textarea", msg, tab=tab)
    browser("click", SEND_ICON, tab=tab)
    wait_for(lambda: js(tab, f"document.body.innerText.includes({json.dumps(msg)})"), "the posted message")
    if js(tab, "(document.querySelector('textarea')||{}).value"):
        raise AssertionError("message box not cleared after send")
    evidence["posted"] = msg
    return evidence


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--url", default=DEFAULT_URL)
    ap.add_argument("--topic", default="General", help="topic to post the test message in")
    ap.add_argument("--expect-topics", nargs="*", default=["Team"], help="topics that must be listed")
    args = ap.parse_args()
    try:
        evidence = run(args)
    except Blocked as e:
        print(json.dumps({"result": "BLOCKED", "reason": str(e)}))
        return 2
    except Exception as e:  # noqa: BLE001 - any failure is a failed journey
        print(json.dumps({"result": "FAIL", "reason": str(e)}))
        return 1
    print(json.dumps({"result": "PASS", **evidence}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
