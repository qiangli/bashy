"""Check the actual script/command/skill paths without touching team data."""
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

with tempfile.TemporaryDirectory(prefix="brainstorm-duality-") as root:
    root = Path(root)
    data = root / "board"
    sessions = data / "sessions"
    env = dict(os.environ, BRAINSTORM_DATA=str(data), XDG_CACHE_HOME=str(root / "cache"))
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]
    env["BRAINSTORM_ADDR"] = f"127.0.0.1:{port}"
    base = f"http://127.0.0.1:{port}"

    def post(path, body, key=None):
        data_bytes = json.dumps(body).encode()
        headers = {"Content-Type": "application/json"}
        if key:
            headers["X-Facilitator-Key"] = key
        req = urllib.request.Request(base + path, data=data_bytes, headers=headers)
        with urllib.request.urlopen(req, timeout=5) as r:
            return json.load(r)

    def run(*args, extra=None, success=True):
        p = subprocess.run([BASHY, *args], env=dict(env, **(extra or {})),
                           text=True, capture_output=True, timeout=60)
        assert (p.returncode == 0) == success, (args, p.returncode, p.stdout, p.stderr)
        return p

    with (root / "server.log").open("w") as log:
        server = subprocess.Popen([BASHY, "brainstorm", "serve"], env=env,
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
            body = json.dumps({"action": "add", "text": "duality fixture", "column": 0}).encode()
            with urllib.request.urlopen(urllib.request.Request(base + "/api/action", data=body,
                                        headers={"Content-Type": "application/json"})) as r:
                assert json.load(r)["total"] == 1
            script = json.loads(run("--bashsharp", str(HERE / "brainstorm.bsh"), "status").stdout)
            command = json.loads(run("brainstorm", "status").stdout)
            assert script == command and command["sessions"][0]["cards"] == 1
            receipt = json.loads(run("skill", "run", "brainstorm", "--json").stdout)
            assert receipt["valid"] is True
            guidance = run("--bashsharp", str(HERE / "brainstorm.bsh"), "--skill").stdout
            assert guidance.strip() == run("skill", "show", "brainstorm").stdout.strip()
            expected = (sessions / "main.json").read_bytes()
            for args in [("--bashsharp", str(HERE / "brainstorm.bsh"), "backup"), ("brainstorm", "backup")]:
                snapshot = Path(run(*args).stdout.strip())
                assert (snapshot / "main.json").read_bytes() == expected
                assert snapshot.stat().st_mode & 0o077 == 0
                assert (snapshot / "main.json").stat().st_mode & 0o077 == 0
            before = set((data / "backups").iterdir())
            receipt = json.loads(run("skill", "run", "brainstorm", "--json",
                                     extra={"BRAINSTORM_ACTION": "backup"}).stdout)
            assert receipt["valid"] is True
            created = set((data / "backups").iterdir()) - before
            assert len(created) == 1 and (created.pop() / "main.json").read_bytes() == expected
            # Desk door: the CLI verbs and the HTTP action are one door.
            made = run("brainstorm", "new", "--id", "duality-desk", "--body", "duality-secret").stdout.splitlines()
            desk_id, desk_url, desk_status = made[0].split()
            desk_key = made[1].split()[1]
            keyed = {"BRAINSTORM_KEY": desk_key}
            opened = run("brainstorm", "open", desk_id, extra=keyed).stdout
            assert "duality-secret" not in opened and opened.split() == [desk_id, desk_url, desk_status], opened
            http_open = post("/api/action", {"action": "open", "id": desk_id}, desk_key)
            assert [http_open["id"], http_open["url"], http_open["status"]] == [desk_id, desk_url, desk_status], http_open
            assert "duality-secret" not in json.dumps(http_open)
            assert "duality-secret" in run("brainstorm", "get", desk_id, extra=keyed).stdout
            assert desk_id in run("brainstorm", "list", extra=keyed).stdout
            run("brainstorm", "open", "no-such-session", extra=keyed, success=False)
            run("brainstorm", "unknown-action", success=False)
            missing = {"BRAINSTORM_DATA": str(root / "missing")}
            run("brainstorm", "status", extra=missing, success=False)
            failed = run("skill", "run", "brainstorm", "--json", extra=missing, success=False)
            # This Bashy version aborts a failed step before emitting a receipt.
            # A later version may emit a failing receipt; neither may attest success.
            if failed.stdout.strip():
                assert json.loads(failed.stdout)["valid"] is False
            assert "no such file" in failed.stderr.lower(), failed.stderr
            assert (sessions / "main.json").read_bytes() == expected
            print("PASS: script/command/skill parity, skill projection, verified backups, failed step cannot attest success; team sessions untouched")
        finally:
            try:
                os.killpg(server.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            server.wait(timeout=10)
