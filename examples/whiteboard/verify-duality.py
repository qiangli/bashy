"""Check the actual script/command/skill paths without touching group boards."""
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time
import urllib.request

HERE = Path(__file__).resolve().parent
BASHY = os.environ.get("BASHY_BIN", "bashy")

with tempfile.TemporaryDirectory(prefix="whiteboard-duality-") as root:
    root = Path(root)
    data = root / "boards"
    env = dict(os.environ, WHITEBOARD_DATA=str(data), XDG_CACHE_HOME=str(root / "cache"))
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]
    env["WHITEBOARD_ADDR"] = f"127.0.0.1:{port}"
    base = f"http://127.0.0.1:{port}"

    def run(*args, extra=None, success=True):
        p = subprocess.run([BASHY, *args], env=dict(env, **(extra or {})),
                           text=True, capture_output=True, timeout=60)
        assert (p.returncode == 0) == success, (args, p.returncode, p.stdout, p.stderr)
        return p

    def post(path, body, key=None):
        data_bytes = json.dumps(body).encode()
        headers = {"Content-Type": "application/json"}
        if key:
            headers["X-Facilitator-Key"] = key
        req = urllib.request.Request(base + path, data=data_bytes, headers=headers)
        with urllib.request.urlopen(req, timeout=5) as r:
            return json.load(r)

    with (root / "server.log").open("w") as log:
        server = subprocess.Popen([BASHY, "whiteboard", "serve"], env=env,
                                  stdout=log, stderr=log, start_new_session=True)
        try:
            for _ in range(120):
                try:
                    with urllib.request.urlopen(base + "/healthz", timeout=1) as r:
                        assert json.load(r)["ok"]
                    break
                except Exception:
                    if server.poll() is not None:
                        raise AssertionError((root / "server.log").read_text())
                    time.sleep(.5)
            else:
                raise AssertionError("Temporary server did not start")
            state = post("/api/action", {"action": "stroke", "board": "main",
                                         "points": [{"x": 10, "y": 10}, {"x": 60, "y": 40}],
                                         "color": "#1f5c53", "width": 4})
            assert len(state["strokes"]) == 1, state
            script = json.loads(run("--bashsharp", str(HERE / "whiteboard.bsh"), "status").stdout)
            command = json.loads(run("whiteboard", "status").stdout)
            assert script == command
            assert [b for b in command["boards"] if b["board"] == "main"][0]["strokes"] == 1
            receipt = json.loads(run("skill", "run", "whiteboard", "--json").stdout)
            assert receipt["valid"] is True
            guidance = run("--bashsharp", str(HERE / "whiteboard.bsh"), "--skill").stdout
            assert guidance.strip() == run("skill", "show", "whiteboard").stdout.strip()
            expected = (data / "boards" / "main.json").read_bytes()
            for args in [("--bashsharp", str(HERE / "whiteboard.bsh"), "backup"), ("whiteboard", "backup")]:
                snapshot = Path(run(*args).stdout.strip())
                assert (snapshot / "main.json").read_bytes() == expected
                assert snapshot.stat().st_mode & 0o077 == 0
            before = set((data / "backups").iterdir())
            receipt = json.loads(run("skill", "run", "whiteboard", "--json",
                                     extra={"WHITEBOARD_ACTION": "backup"}).stdout)
            assert receipt["valid"] is True
            created = set((data / "backups").iterdir()) - before
            assert len(created) == 1
            assert (created.pop() / "main.json").read_bytes() == expected
            # Desk door: the CLI verbs and the HTTP action are one door.
            made = run("whiteboard", "new", "--id", "duality-desk", "--body", "duality-secret").stdout.splitlines()
            desk_id, desk_url, desk_status = made[0].split()
            desk_key = made[1].split()[1]
            keyed = {"WHITEBOARD_KEY": desk_key}
            opened = run("whiteboard", "open", desk_id, extra=keyed).stdout
            assert "duality-secret" not in opened and opened.split() == [desk_id, desk_url, desk_status], opened
            http_open = post("/api/action", {"action": "open", "id": desk_id}, desk_key)
            assert [http_open["id"], http_open["url"], http_open["status"]] == [desk_id, desk_url, desk_status], http_open
            assert "duality-secret" not in json.dumps(http_open)
            assert "duality-secret" in run("whiteboard", "get", desk_id, extra=keyed).stdout
            assert desk_id in run("whiteboard", "list", extra=keyed).stdout
            run("whiteboard", "open", "no-such-board", extra=keyed, success=False)
            run("whiteboard", "unknown-action", success=False)
            missing = {"WHITEBOARD_DATA": str(root / "missing")}
            run("whiteboard", "status", extra=missing, success=False)
            failed = run("skill", "run", "whiteboard", "--json", extra=missing, success=False)
            # This Bashy version aborts a failed step before emitting a receipt.
            # A later version may emit a failing receipt; neither may attest success.
            if failed.stdout.strip():
                assert json.loads(failed.stdout)["valid"] is False
            assert "no such file" in failed.stderr.lower(), failed.stderr
            assert (data / "boards" / "main.json").read_bytes() == expected
            print("PASS: script/command/skill parity, skill projection, verified backups, failed step cannot attest success; group boards untouched")
        finally:
            try:
                os.killpg(server.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            server.wait(timeout=10)
