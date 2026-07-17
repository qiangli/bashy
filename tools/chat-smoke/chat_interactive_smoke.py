#!/usr/bin/env python3
"""chat-interactive smoke — drive `bashy chat` under a REAL pty and assert the
whole governed-launcher contract with a real agent:

  1. native launch    — the agent's own TUI comes up under the pty (governed)
  2. registered       — it appears on the live-sessions board from ANOTHER process
  3. steered          — `bashy chat steer <id>` delivers a frame to its ctlsock
  4. capture tee       — the interactive session is ALSO captured (observable/attachable)
  5. deregistered      — clean teardown removes it from the board

This is a plumbing smoke, not a model test: it never requires the agent to
finish a turn (a native TUI's raw-ANSI redraws don't linearise, and a turn costs
tokens). It launches --read-only for least privilege.

Exit: 0 = PASS · 1 = FAIL · 77 = SKIP (no installed agent / no pty / no bashy).
Override the agent by passing it as argv[1] or $CHAT_SMOKE_AGENT.
"""
import glob
import json
import os
import pty
import re
import select
import shutil
import signal
import subprocess
import sys
import time

SKIP = 77
BASHY = os.environ.get("BASHY") or shutil.which("bashy") or "./bin/bashy"

# Third-party tools first (kimi/deepseek keys) — never seat the caller's own
# subscription agent by default. Any installed one will do; the smoke tests
# bashy's plumbing, not the model.
PREFER = [
    "opencode-kimi-k3", "opencode-deepseek-v4-pro", "opencode-kimi-k2.7-code",
    "opencode-kimi-k2.6", "agy-gemini3.1", "codex-gpt-5.5",
]


def flush(*a):
    print(*a)
    sys.stdout.flush()


def sh(*args, t=20):
    env = dict(os.environ, BASHY_HINTS="off")
    try:
        r = subprocess.run([BASHY, *args], capture_output=True, text=True, timeout=t, env=env)
        return r.stdout + r.stderr  # `chat steer` confirms on stderr
    except Exception as e:
        return f"<err {e}>"


def tool_of(agent):
    # opencode-kimi-k3 -> opencode ; agy-gemini3.1 -> agy ; codex-gpt-5.5 -> codex
    return agent.split("-", 1)[0]


def pick_agent():
    forced = (sys.argv[1] if len(sys.argv) > 1 else "") or os.environ.get("CHAT_SMOKE_AGENT", "")
    listing = sh("agents", "list")
    operable = set()
    for line in listing.splitlines():
        parts = line.split()
        # NAME NICK BAND TOOL MODEL RELIAB RESOLVES RING — RESOLVES is second-last
        if len(parts) >= 8 and parts[-2] == "yes":
            operable.add(parts[0])
    if forced:
        return forced if (forced in operable or not operable) else forced
    for a in PREFER + sorted(operable):
        if a in operable and shutil.which(tool_of(a)):
            return a
    return None


def clean():
    subprocess.run(["pkill", "-9", "-f", "bashy chat --agent"], capture_output=True)
    for t in ("opencode --model", "agy ", "codex "):
        subprocess.run(["pkill", "-9", "-f", t], capture_output=True)


def main():
    if not (BASHY and (shutil.which(BASHY) or os.path.exists(BASHY))):
        flush("SKIP: bashy not found (set $BASHY or install it)")
        return SKIP
    agent = pick_agent()
    if not agent:
        flush("SKIP: no operable third-party agent installed")
        return SKIP
    tool = tool_of(agent)
    if not shutil.which(tool):
        flush(f"SKIP: agent {agent} selected but its tool {tool!r} is not on PATH")
        return SKIP

    flush(f"chat-interactive smoke: agent={agent} tool={tool} bashy={BASHY}")
    clean()

    pid, fd = pty.fork()
    if pid == 0:
        os.environ["BASHY_HINTS"] = "off"
        os.execvp(BASHY, [BASHY, "chat", "--agent", agent, "--read-only"])
        os._exit(127)

    buf = b""

    def pump(dur):
        nonlocal buf
        end = time.time() + dur
        while time.time() < end:
            r, _, _ = select.select([fd], [], [], 0.3)
            if fd in r:
                try:
                    c = os.read(fd, 4096)
                except OSError:
                    return
                if not c:
                    return
                buf += c

    checks = {}
    # 1) let the native TUI come up + register
    pump(9)
    checks["native_launch"] = len(buf) > 200  # the TUI wrote SOMETHING to the pty

    board = sh("chat", "sessions")
    m = re.search(r"^(%s\S*)" % re.escape(agent), board, re.M)
    sid = m.group(1) if m else None
    checks["registered"] = bool(sid)

    if sid:
        steer = sh("chat", "steer", sid, "noop smoke; no action needed")
        checks["steered"] = "steered" in steer
        # 4) capture tee: the session's log_path exists and has grown
        sz = -1
        for rf in glob.glob(os.path.expanduser("~/.bashy/sessions/*.json")):
            try:
                d = json.load(open(rf))
            except Exception:
                continue
            if d.get("id") == sid:
                lp = d.get("log_path", "")
                sz = os.path.getsize(lp) if lp and os.path.exists(lp) else -1
        checks["capture_tee"] = sz > 0
        sh("chat", "interrupt", sid)
    pump(1)

    # 5) teardown — Ctrl-C then hard kill, confirm deregistration
    try:
        os.write(fd, b"\x03\x03")
        time.sleep(0.6)
        os.kill(pid, signal.SIGKILL)
        os.waitpid(pid, 0)
    except OSError:
        pass
    time.sleep(1)
    board2 = sh("chat", "sessions")
    checks["deregistered"] = bool(sid) and sid not in board2
    clean()

    ok = all(checks.values())
    for k, v in checks.items():
        flush(f"  [{'PASS' if v else 'FAIL'}] {k}")
    flush("RESULT:", "PASS" if ok else "FAIL")
    return 0 if ok else 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    finally:
        clean()
