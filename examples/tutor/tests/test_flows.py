"""One-command flows: `tutor demo` and `tutor go`, via the verb and via bashy dag.

Each flow runs on a pty (a human's terminal) with a stub opener; the real
stores here are the TempBashy ones, so "real HOME unchanged" is checked on them.
"""
import json
import os
import pty
import select
import signal
import socket
import subprocess
import time
import urllib.request
from pathlib import Path

from tests.helpers import HERE, TempBashy

_SERVER = []


def setUpModule():
    out = subprocess.run(["bash", str(HERE / "server" / "run.sh"), "build"], env=dict(os.environ, BASHY_HINTS="off"),
                         text=True, capture_output=True, timeout=300)
    assert out.returncode == 0, out.stderr
    cache = os.environ.get("XDG_CACHE_HOME") or os.path.join(os.path.expanduser("~"), ".cache")
    _SERVER.append(os.path.join(cache, "bashy", "tutor", "tutor-server"))
    assert os.access(_SERVER[0], os.X_OK), _SERVER[0]


def lan_address():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        s.connect(("10.255.255.255", 1))
        ip = s.getsockname()[0]
    except OSError:
        return None
    finally:
        s.close()
    return None if ip.startswith("127.") else ip


NOISE = ("room", "exec", ".agents", "Library", "skills", ".cache", "claims.lock", ".lock")


def snapshot(root):
    """Stores under root, ignoring bashy's own per-invocation startup files."""
    return sorted(str(p.relative_to(root)) for p in Path(root).rglob("*")
                  if not any(part in NOISE or part.endswith(".lock") for part in p.relative_to(root).parts))


class Flows(TempBashy):
    def setUp(self):
        super().setUp()
        self.opened = self.root / "opened.txt"
        opener = self.root / "opener.sh"
        opener.write_text('#!/bin/sh\necho "$1" >> "%s"\n' % self.opened)
        opener.chmod(0o755)
        os.environ.update(TUTOR_OPEN=str(opener), TUTOR_SERVER_BIN=_SERVER[0])

    def run_flow(self, argv, check):
        """Run argv on a pty until its Ctrl-C hint, call check(lines), then SIGINT."""
        self.opened.unlink(missing_ok=True)
        master, slave = pty.openpty()
        proc = subprocess.Popen(argv, stdout=slave, stderr=slave, stdin=subprocess.DEVNULL,
                                start_new_session=True, env=dict(os.environ))
        os.close(slave)
        buf = b""
        deadline = time.time() + 90
        try:
            while time.time() < deadline and b"Ctrl-C to stop" not in buf:
                if select.select([master], [], [], 1)[0]:
                    try:
                        chunk = os.read(master, 4096)
                    except OSError:
                        break
                    buf += chunk
            lines = [l.strip() for l in buf.decode().splitlines() if l.strip() and not l.startswith("==> ")]
            self.assertTrue(any("Ctrl-C to stop" in l for l in lines), buf.decode())
            check(lines)
        finally:
            try:
                os.killpg(proc.pid, signal.SIGINT)
            except ProcessLookupError:
                pass
            try:
                proc.wait(30)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid, signal.SIGKILL)
                proc.wait()
            os.close(master)
        time.sleep(0.5)
        return lines

    def assert_up(self, display):
        base = display.rstrip("/")
        port = display.rsplit(":", 1)[1].split("/")[0]
        local = "http://127.0.0.1:%s" % port
        with urllib.request.urlopen(local + "/healthz", timeout=5) as r:
            self.assertTrue(json.load(r)["ok"])
        with urllib.request.urlopen(local + "/api/display", timeout=10) as r:
            state = json.load(r)
        statuses = [s["status"] for s in state["steps"]]
        self.assertIn("doing", statuses[:1] + statuses, statuses)
        self.assertEqual(self.opened.read_text().split(), [display])
        return port, base

    def assert_server_gone(self, port):
        with self.assertRaises(OSError):
            urllib.request.urlopen("http://127.0.0.1:%s/healthz" % port, timeout=2)

    def assert_links(self, lines, phones):
        urls = [l for l in lines if l.startswith("http")]
        self.assertEqual(len(urls), 1 + phones, lines)
        lan = lan_address()
        if lan:
            for u in urls:
                self.assertNotIn("127.0.0.1", u)
        return urls

    def test_demo_verb_and_dag(self):
        env_dirs = [self.root / "home", self.root / "xdg", self.root / "coord", self.root / "room"]
        for d in env_dirs:
            d.mkdir(exist_ok=True)
        before = snapshot(self.root)
        verbs = [["bashy", "--bashsharp", str(HERE / "tutor.bsh"), "demo"],
                 ["bashy", "dag", "--plain", str(HERE / "dag.md"), "demo", "TUTOR_DIR=%s" % HERE]]
        for argv in verbs:
            with self.subTest(argv=argv[1:3]):
                seen = {}

                def check(lines):
                    urls = self.assert_links(lines, 1)
                    self.assertTrue(urls[0].endswith("/"), urls)
                    self.assertIn("/s/", urls[1])
                    seen["port"], _ = self.assert_up(urls[0])
                    self.assertEqual(len(lines), 3, lines)

                self.run_flow(argv, check)
                self.assert_server_gone(seen["port"])
                self.opened.unlink()
                self.assertEqual(snapshot(self.root), before)

    def test_demo_links_stream_when_stdout_is_a_pipe(self):
        proc = subprocess.Popen(["bashy", "--bashsharp", str(HERE / "tutor.bsh"), "demo"],
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                stdin=subprocess.DEVNULL, start_new_session=True,
                                env=dict(os.environ))
        try:
            out = b""
            deadline = time.time() + 90
            while time.time() < deadline and b"Ctrl-C to stop" not in out:
                if select.select([proc.stdout], [], [], 1)[0]:
                    chunk = os.read(proc.stdout.fileno(), 4096)
                    if not chunk:
                        break
                    out += chunk
            self.assertIn(b"Ctrl-C to stop", out, out.decode())
        finally:
            try:
                os.killpg(proc.pid, signal.SIGINT)
            except ProcessLookupError:
                pass
            proc.wait(timeout=30)

    def test_go_verb_and_dag(self):
        goal = "Build a small thing"
        verbs = [["bashy", "--bashsharp", str(HERE / "tutor.bsh"), "go", goal,
                  "--people", "alice,bob", "--minutes", "30"],
                 ["bashy", "dag", "--plain", str(HERE / "dag.md"), "go", "TUTOR_DIR=%s" % HERE,
                  "GOAL=" + goal, "PEOPLE=alice,bob", "MINUTES=30"]]
        for argv in verbs:
            with self.subTest(argv=argv[1:3]):
                seen = {}

                def check(lines):
                    text = "\n".join(lines)
                    sprint = int(next(l for l in lines if l.startswith("Session:")).split()[2])
                    seen["sprint"] = sprint
                    self.assertIn("Join alice:", text)
                    self.assertIn("Join bob:", text)
                    self.assertIn("(keep private)", text)
                    self.assertIn("session data stays", text)
                    display = next(l for l in lines if l.startswith("Display:")).split()[1]
                    seen["port"], _ = self.assert_up(display)
                    status = json.loads(self.tutor("status", str(sprint), "--json").stdout)
                    self.assertEqual(status["current"]["index"], 1)

                self.run_flow(argv, check)
                self.assert_server_gone(seen["port"])
                self.opened.unlink()
                status = json.loads(self.tutor("status", str(seen["sprint"]), "--json").stdout)
                self.assertEqual(status["sprint"], seen["sprint"])

    def test_go_requires_goal_and_people(self):
        for args in (["go"], ["go", "x"], ["go", "--people", "a"]):
            self.assertEqual(self.tutor(*args, check=False).returncode, 2)
