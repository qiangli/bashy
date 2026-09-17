"""Real-PTY interaction and real-signal interruption fixtures.

These drive the ACTUAL cli.py process — not a mock — over a pseudo-terminal
(so `sys.stdin.isatty()` is genuinely True and the prompter reads real tty
bytes) and deliver a REAL SIGINT. They assert on the persisted trajectory and
the process exit code rather than parsing the mixed pty stream.

Subcommands:
    pty-confirm   confirm mode: approve each action over a real tty -> Submitted
    pty-human     human mode: type the submit command over a real tty -> Submitted
    signal        yolo run, real SIGINT during a long action -> UserInterruption,
                  trajectory saved, child process group reaped (no orphan)

Requires BASHY_BIN (or bashy on PATH) and python3. Offline; no model calls.
"""

from __future__ import annotations

import json
import os
import pty
import select
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
EXAMPLE = HERE.parent
HARNESS = EXAMPLE / "harness"
SCENARIOS = HERE / "scenarios"

PROMPT_MARKERS = (b"Execute", b"finish", b"> ", b"What do you want")


def _env() -> dict:
    env = dict(os.environ)
    env["PYTHONPATH"] = str(HARNESS)
    env["BASHY_HINTS"] = "off"
    env.setdefault("PYTHON_BIN", sys.executable)
    return env


def _run_pty(args: list[str], responder, *, timeout: float = 30.0) -> int:
    """Run cli.py under a pty; call responder(chunk)->bytes|None to feed input."""
    master, slave = pty.openpty()
    proc = subprocess.Popen(
        [sys.executable, "-m", "minisweagent_bounded.cli", *args],
        stdin=slave, stdout=slave, stderr=slave, env=_env(), cwd=str(HARNESS),
        start_new_session=True,
    )
    os.close(slave)
    deadline = time.time() + timeout
    try:
        while True:
            if proc.poll() is not None:
                break
            if time.time() > deadline:
                proc.kill()
                raise TimeoutError("pty interaction timed out")
            r, _, _ = select.select([master], [], [], 0.2)
            if master in r:
                try:
                    chunk = os.read(master, 4096)
                except OSError:
                    break
                if not chunk:
                    break
                reply = responder(chunk)
                if reply:
                    os.write(master, reply)
    finally:
        os.close(master)
    return proc.wait()


def _approver(chunk: bytes):
    # Approve/continue whenever a prompt appears (Enter).
    if any(m in chunk for m in PROMPT_MARKERS):
        return b"\n"
    return None


def _load_traj(path: Path) -> dict:
    return json.loads(path.read_text())


def pty_confirm() -> int:
    with tempfile.TemporaryDirectory() as tmp:
        traj = Path(tmp) / "traj.json"
        rc = _run_pty(
            ["--scenario", str(SCENARIOS / "submit-success.json"), "--mode", "confirm",
             "--exit-immediately", "-o", str(traj)],
            _approver,
        )
        data = _load_traj(traj)
        status = data["info"]["exit_status"]
        ok = rc == 0 and status == "Submitted"
        print(f"pty-confirm: rc={rc} exit_status={status} -> {'PASS' if ok else 'FAIL'}")
        return 0 if ok else 1


def pty_human() -> int:
    # In human mode the user types commands. Feed the submit command on the '>'.
    submit_cmd = b"echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\n"
    sent = {"done": False}

    def responder(chunk: bytes):
        if b">" in chunk and not sent["done"]:
            sent["done"] = True
            return submit_cmd
        return None

    with tempfile.TemporaryDirectory() as tmp:
        traj = Path(tmp) / "traj.json"
        rc = _run_pty(
            ["-t", "submit immediately", "--mode", "human", "--exit-immediately",
             "--model-class", "replay", "--replay", str(SCENARIOS / "noop-steps.json"),
             "-o", str(traj)],
            responder,
        )
        data = _load_traj(traj)
        status = data["info"]["exit_status"]
        ok = rc == 0 and status == "Submitted"
        print(f"pty-human: rc={rc} exit_status={status} -> {'PASS' if ok else 'FAIL'}")
        return 0 if ok else 1


def signal_case() -> int:
    marker = "sleep 41.5"  # unique so we can detect an orphan
    with tempfile.TemporaryDirectory() as tmp:
        traj = Path(tmp) / "traj.json"
        proc = subprocess.Popen(
            [sys.executable, "-m", "minisweagent_bounded.cli",
             "--scenario", str(SCENARIOS / "interrupt-sleep.json"), "-y", "-o", str(traj)],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            env=_env(), cwd=str(HARNESS), start_new_session=True,
        )
        time.sleep(1.5)  # let the sleep action start
        proc.send_signal(signal.SIGINT)  # a REAL SIGINT
        try:
            rc = proc.wait(timeout=15)
        except subprocess.TimeoutExpired:
            proc.kill()
            print("signal: FAIL (process did not exit after SIGINT)")
            return 1
        time.sleep(0.5)
        # No orphaned child running our unique sleep.
        pg = subprocess.run(["pgrep", "-f", marker], capture_output=True, text=True)
        orphan = pg.returncode == 0 and pg.stdout.strip() != ""
        status = _load_traj(traj)["info"]["exit_status"] if traj.exists() else "<no trajectory>"
        ok = rc == EXIT_INTERRUPT and status == "UserInterruption" and not orphan
        print(f"signal: rc={rc} exit_status={status} orphan={orphan} -> {'PASS' if ok else 'FAIL'}")
        return 0 if ok else 1


EXIT_INTERRUPT = 130


def main(argv: list[str]) -> int:
    if not argv:
        print("usage: interactive_check.py {pty-confirm|pty-human|signal|all}", file=sys.stderr)
        return 2
    what = argv[0]
    cases = {"pty-confirm": pty_confirm, "pty-human": pty_human, "signal": signal_case}
    if what == "all":
        return max(fn() for fn in cases.values())
    if what in cases:
        return cases[what]()
    print(f"unknown case: {what}", file=sys.stderr)
    return 2


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
