#!/usr/bin/env python3
"""Field-loop e2e: incident heal, owner feed, and scheduled feed with fakes.

Run only on a claimed gate lane (FIELD_LOOP_GATE=1). Sandbox with fakes:
a deterministic fake shaping agent (fixtures/tools/codex), an issues file for
shape-feed --source file:..., no network, no real GitHub, no browser.
Writes one evidence log (--evidence, must be a fresh path) and fails closed:
any missing prerequisite or failed check exits nonzero.

Scenarios (F4):
  (a) injected 5xx on form -> incident repair heals -> activated ->
      `bashy shape auto log` shows trigger/attempt/verdict/activation ->
      previous revision restorable;
  (b) owner issue via shape-feed --apply -> todo (origin feed, reporter) ->
      wish --source feed:ID -> approve -> activated; non-owner skipped;
  (c) same feed from `bashy schedule` once; after `bashy shape auto off`
      a second run files nothing and the log shows the refusal.
Interfaces written against F3b: `shape wish --source KIND:REF`, the
off-switch/log on the wish path, and shape-feed calling `shape approve`.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
PASS = "PASS field loop"
APP = "form"
OWNER = os.environ.get("FIELD_LOOP_OWNER", "field-owner")
AGENT = "codex:gpt-field-loop"
WAIT = float(os.environ.get("FIELD_LOOP_WAIT_SECONDS", "120"))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", type=Path, required=True,
                        help="new raw log file (must not exist)")
    args = parser.parse_args()
    if os.environ.get("FIELD_LOOP_GATE") != "1":
        parser.error("run on a claimed gate lane with FIELD_LOOP_GATE=1")
    binary = Path(os.environ.get(
        "FIELD_LOOP_BASHY_BIN", str(ROOT / "bin" / "bashy"))).resolve()
    if not binary.is_file():
        parser.error(f"bashy binary not found: {binary} "
                     "(set FIELD_LOOP_BASHY_BIN)")
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    with args.evidence.open("x") as log:
        def record(msg):
            print(msg, file=log, flush=True)
            print(msg, flush=True)

        def check(cond, msg):
            if not cond:
                raise AssertionError(msg)
            record(f"ok - {msg}")

        def run(*argv, check_rc=True, timeout=None):
            record("+ " + " ".join(str(a) for a in argv))
            proc = subprocess.run([str(a) for a in argv], cwd=str(home),
                                  stdout=subprocess.PIPE,
                                  stderr=subprocess.STDOUT, text=True,
                                  timeout=timeout or WAIT)
            record(proc.stdout.rstrip())
            if check_rc and proc.returncode:
                raise AssertionError(
                    f"command failed ({proc.returncode}): {argv[0]}")
            return proc

        def bashy(*argv, **kw):
            return run(binary, *argv, **kw)

        def bashy_json(*argv):
            out = bashy(*argv, "--json").stdout
            # The last JSON document in the output, single-line or
            # pretty-printed (notes may precede it).
            lines = out.splitlines()
            dec = json.JSONDecoder()
            for i in range(len(lines) - 1, -1, -1):
                # Top-level documents start in column 0; indented braces
                # belong to a nested value.
                if lines[i].startswith(("{", "[")):
                    try:
                        doc, _ = dec.raw_decode("\n".join(lines[i:]))
                        return doc
                    except json.JSONDecodeError:
                        continue
            raise AssertionError(f"no JSON in output of {argv}: {out!r}")

        for path in (Path(__file__), HERE / "fixtures/issues.json",
                     ROOT / "dag.md"):
            record(f"{path.name} sha256="
                   f"{hashlib.sha256(path.read_bytes()).hexdigest()}")
        record(f"bashy sha256="
               f"{hashlib.sha256(binary.read_bytes()).hexdigest()}")

        go_env = json.loads(subprocess.run(
            ["go", "env", "-json", "GOMODCACHE", "GOCACHE"],
            stdout=subprocess.PIPE, text=True, check=True).stdout)
        home = Path(tempfile.mkdtemp(prefix="field-loop-home-"))
        record(f"throwaway HOME={home}")
        fixture_bin = HERE / "fixtures" / "tools"
        env = dict(os.environ)
        env.update({
            "HOME": str(home), "BASHY_HOME": str(home / ".bashy"),
            "BASHY_HINTS": "off", "BROWSER": "/usr/bin/true",
            "PATH": f"{fixture_bin}:{home}/bin:{os.environ.get('PATH', '')}",
            "GIT_TERMINAL_PROMPT": "0", "GOWORK": "off",
            # Keep the venue's Go caches: a throwaway HOME would otherwise
            # hide them and the form server build would refetch everything.
            "GOMODCACHE": go_env["GOMODCACHE"], "GOCACHE": go_env["GOCACHE"],
            "BASHY_PRINCIPAL": OWNER, "FIELD_LOOP_OWNER": OWNER,
        })
        os.environ.clear()
        os.environ.update(env)
        for name in ("codex", "open", "xdg-open"):
            os.chmod(fixture_bin / name, 0o755)
        check(shutil.which("codex") == str(fixture_bin / "codex"),
              "fake shaping agent (codex stub) found on PATH")
        check(shutil.which("open") == str(fixture_bin / "open"),
              "no-op open on PATH (no browser)")

        run("git", "config", "--global", "user.name", "Field Loop Fixture")
        run("git", "config", "--global", "user.email", f"{OWNER}@example.invalid")
        bashy("tools", "set", "codex", "--set",
              f"cli.binary={fixture_bin / 'codex'}")
        preflight = bashy_json("delegate", "--dry-run",
                               "--agent", AGENT, "binding preflight")
        check(preflight.get("agent") == str(fixture_bin / "codex"),
              f"{AGENT} resolves to the fake agent")

        # Register the real form shapeware app from a throwaway git copy so
        # candidate clones have a clean base revision.
        # form/register.sh sources ../register-modules.sh, which publishes
        # the shared desk modules (kit, chart) to the local module proxy, so
        # the copy keeps form's desk siblings next to it.
        desk = home / "desk"
        desk.mkdir()
        shutil.copy2(ROOT / "examples/desk/register-modules.sh", desk)
        for mod in ("kit", "chart"):
            shutil.copytree(ROOT / "examples/desk" / mod, desk / mod)
        src = desk / "form"
        shutil.copytree(ROOT / "examples/desk/form", src,
                        ignore=shutil.ignore_patterns(
                            "node_modules", ".git", "*.test"))
        run("git", "-C", src, "init", "-q")
        run("git", "-C", src, "add", "-A")
        run("git", "-C", src, "commit", "-qm", "field-loop base")
        os.environ["BASHY_BIN"] = str(binary)
        run("bash", src / "register.sh", APP, timeout=WAIT)
        status = bashy_json("shape", "status", APP, "--user", OWNER)
        base_rev = status.get("active") or status.get("revision") or ""
        check(bool(base_rev), f"form registered with active base {base_rev}")
        # Incident repair ships as a patch release on top of a base release;
        # a freshly registered app has none, so cut v0 of the base first.
        bashy("src", "release", APP, base_rev, "--user", OWNER, "--shared")

        auto_log = lambda: bashy("shape", "auto", "log").stdout  # noqa: E731

        def todo_items():
            out = bashy("todo", "list", "--all", "--json").stdout
            items = []
            for line in out.splitlines():
                line = line.strip()
                if not line.startswith("{"):
                    continue
                node = json.loads(line)
                if node.get("t") == "node":
                    items.append(node.get("attrs", {}))
            return items

        # --- (a) injected 5xx -> incident repair -> activated -> restorable
        record("== (a) incident repair heals an injected 5xx ==")
        # The console's loopback caller is the host operator; incident
        # repair activates that principal's version.
        import getpass
        OPERATOR = "os:" + getpass.getuser()
        form_bsh = src / "form.bsh"
        form_bsh.write_text(form_bsh.read_text()
                            + "\n# FIELD_LOOP_5XX_MARKER injected 5xx\n")
        run("git", "-C", src, "add", "-A")
        run("git", "-C", src, "commit", "-qm", "inject 5xx marker")
        # Incident repair runs inside the console, where the shaping
        # delegate, verifier and activator live (the local CLI only records
        # a report). Serve the console on loopback (the host operator, no
        # pairing) and report the failure on the route the page's own error
        # capture uses.
        import socket
        import urllib.request
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        console_log = (home / "console.log").open("w")
        console = subprocess.Popen(
            [str(binary), "app", "serve", "--port", str(port),
             "--shape-agent", AGENT],
            stdout=console_log, stderr=subprocess.STDOUT)

        def stop_console():
            if console.poll() is None:
                console.kill()
            print("console log tail:\n"
                  + (home / "console.log").read_text()[-3000:])
        import atexit
        atexit.register(stop_console)
        base_url = f"http://127.0.0.1:{port}"
        deadline = time.monotonic() + WAIT
        while time.monotonic() < deadline:
            try:
                urllib.request.urlopen(base_url + "/", timeout=2).read()
                break
            except Exception:  # noqa: BLE001 - not up yet
                time.sleep(1)
        check(console.poll() is None, f"console serving on {base_url}")
        report = json.dumps({
            "kind": "script", "path": "/",
            "message": "injected 5xx: form page returns 500 "
                       "(FIELD_LOOP_5XX_MARKER)"}).encode()
        req = urllib.request.Request(
            f"{base_url}/api/shape/{APP}/incidents", data=report,
            headers={"Content-Type": "application/json"}, method="POST")
        with urllib.request.urlopen(req, timeout=30) as resp:
            record(f"incident report -> HTTP {resp.status}: "
                   f"{resp.read().decode(errors='replace')[:400]}")
        deadline = time.monotonic() + WAIT
        healed = ""
        while time.monotonic() < deadline:
            healed = bashy_json("shape", "status", APP,
                                "--user", OPERATOR).get("active", "")
            if healed and healed != base_rev:
                break
            time.sleep(2)
        if not healed or healed == base_rev:
            record("diagnostics: incidents and decision log")
            bashy("shape", "incident", APP, "--user", OPERATOR, "--json",
                  check_rc=False)
            bashy("shape", "auto", "log", check_rc=False)
        check(bool(healed) and healed != base_rev,
              f"incident repair activated a healed revision ({healed})")
        log_a = auto_log()
        for event in ("trigger", "attempt", "verdict", "activation"):
            check(event in log_a, f"auto log shows {event} for incident repair")
        console.terminate()
        console.wait(timeout=30)
        bashy("shape", "restore", APP, base_rev, "--user", OPERATOR)
        restored = bashy_json("shape", "status", APP,
                              "--user", OPERATOR).get("active", "")
        check(restored == base_rev,
              f"previous revision restorable (active={restored})")

        # --- (b) owner issue via shape-feed --apply -----------------------
        record("== (b) shape-feed owner issue -> todo -> wish -> activated ==")
        # Register shape-feed in this HOME the way bashy-defaults does.
        bashy("commands", "add", "shape-feed",
              "--set", f"exec.0={binary}", "--set", "exec.1=--bashsharp",
              "--set", f"exec.2={ROOT / 'examples/shape-feed/shape-feed.bsh'}",
              "--set", "effects.0=read", "--set", "effects.1=write",
              "--set", "effects.2=exec", "--set", "effects.3=net")
        issues = HERE / "fixtures" / "issues.json"
        feed = bashy("shape-feed", APP, "--source", f"file:{issues}",
                     "--owner", OWNER, "--agent", AGENT, "--apply",
                     timeout=WAIT)
        check("filed" in feed.stdout or "applied" in feed.stdout,
              "shape-feed --apply filed the owner issue")
        items = todo_items()
        feed_todos = [t for t in items
                      if t.get("origin") == "feed"]
        check(len(feed_todos) == 1,
              f"exactly one feed todo filed (got {len(feed_todos)})")
        check(feed_todos[0].get("reporter") == OWNER,
              "feed todo records the owner as reporter")
        deadline = time.monotonic() + WAIT
        active_b = ""
        while time.monotonic() < deadline:
            active_b = bashy_json("shape", "status", APP,
                                  "--user", OWNER).get("active", "")
            if active_b and active_b != base_rev:
                break
            time.sleep(2)
        check(bool(active_b) and active_b != base_rev,
              f"feed wish verified and activated ({active_b})")
        check("someone-else" not in json.dumps(items),
              "non-owner issue skipped (no todo for someone-else)")

        # --- (c) scheduled feed + off switch ------------------------------
        record("== (c) scheduled feed once, then off stops it ==")
        sched = bashy_json("schedule", "add", "--name", "field-loop-feed",
                           "--every", "24h", "--",
                           str(binary), "shape-feed", APP,
                           "--source", f"file:{issues}",
                           "--owner", OWNER, "--agent", AGENT, "--apply")
        job_id = str(sched.get("id") or sched.get("result", {}).get("id", ""))
        check(bool(job_id), f"schedule job added ({job_id})")
        before = len(todo_items())
        bashy("schedule", "run", job_id, timeout=WAIT)
        bashy("shape", "auto", "off")
        check("off" in bashy("shape", "auto", "status").stdout,
              "unattended shaping switched off")
        bashy("schedule", "run", job_id, timeout=WAIT, check_rc=False)
        after_items = todo_items()
        check(len(after_items) == before,
              "second scheduled run filed nothing after auto off")
        check("refusal" in auto_log(),
              "auto log shows the refusal after auto off")
        record(PASS + f"; evidence {args.evidence}")
        print(PASS)


if __name__ == "__main__":
    main()
