"""Check script/command/skill paths and multi-round E2E in an isolated bashy sandbox."""
import hashlib
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

FACILITATOR_EMAIL = "facilitator@example.test"
ALICE_EMAIL = "alice@example.test"
BOB_EMAIL = "bob@example.test"


def participant_token(email: str) -> str:
    return hashlib.sha256(f"brainstorm-e2e:{email}".encode()).hexdigest()[:48]


with tempfile.TemporaryDirectory(prefix="brainstorm-duality-") as root:
    root = Path(root)
    data = root / "board"
    sessions = data / "sessions"
    sandbox_home = root / "home"
    sandbox_home.mkdir(parents=True, exist_ok=True)
    # Reuse host Go caches so Go builds inside the isolated HOME stay fast and offline-friendly.
    go_env = subprocess.run(
        ["go", "env", "GOMODCACHE", "GOCACHE"],
        text=True, capture_output=True, check=True,
    ).stdout.strip().splitlines()
    env = dict(
        os.environ,
        HOME=str(sandbox_home),
        BASHY_HOME=str(root / "bashy-home"),
        BASHY_FLEET_DIR=str(root / "fleet"),
        BASHY_COORD_DIR=str(root / "coord"),
        BASHY_ROOM_DIR=str(root / "room"),
        BASHY_APPS_DIR=str(root / "apps"),
        XDG_CONFIG_HOME=str(root / "config"),
        XDG_DATA_HOME=str(root / "xdg-data"),
        XDG_CACHE_HOME=str(root / "cache"),
        BRAINSTORM_DATA=str(data),
        BASHY_SPRINT_ANNOUNCE="0",
        GOMODCACHE=go_env[0],
        GOCACHE=go_env[1],
        GIT_AUTHOR_NAME="Facilitator",
        GIT_AUTHOR_EMAIL=FACILITATOR_EMAIL,
        GIT_COMMITTER_NAME="Facilitator",
        GIT_COMMITTER_EMAIL=FACILITATOR_EMAIL,
    )
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]
    env["BRAINSTORM_ADDR"] = f"127.0.0.1:{port}"
    base = f"http://127.0.0.1:{port}"

    def request_json(path, body=None, key=None, email=None):
        headers = {}
        data_bytes = None
        if body is not None:
            data_bytes = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        if key:
            headers["X-Facilitator-Key"] = key
        if email:
            headers["Cookie"] = f"brainstorm-participant={participant_token(email)}"
        req = urllib.request.Request(base + path, data=data_bytes, headers=headers)
        with urllib.request.urlopen(req, timeout=5) as r:
            return json.load(r)

    def post(path, body, key=None, email=None):
        return request_json(path, body=body, key=key, email=email)

    def run(*args, extra=None, success=True):
        p = subprocess.run([BASHY, *args], env=dict(env, **(extra or {})),
                           text=True, capture_output=True, timeout=120)
        assert (p.returncode == 0) == success, (args, p.returncode, p.stdout, p.stderr)
        return p

    # Register brainstorm inside the isolated bashy sandbox.
    run(str(HERE / "register.sh"))

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
            made = run("brainstorm", "add", "--id", "duality-desk", "--body", "duality-secret").stdout.splitlines()
            desk_id, desk_url, desk_status = made[0].split()
            desk_key = made[1].split()[1]
            keyed = {"BRAINSTORM_KEY": desk_key}
            opened = run("brainstorm", "open", desk_id, extra=keyed).stdout
            assert "duality-secret" not in opened and opened.split() == [desk_id, desk_url, desk_status], opened
            http_open = post("/api/action", {"action": "open", "id": desk_id}, desk_key)
            assert [http_open["id"], http_open["url"], http_open["status"]] == [desk_id, desk_url, desk_status], http_open
            assert "duality-secret" not in json.dumps(http_open)
            assert "duality-secret" in run("brainstorm", "show", desk_id, extra=keyed).stdout
            assert desk_id in run("brainstorm", "list", extra=keyed).stdout

            # --- Happy-path E2E: preset seed cards, multi-round ideation/voting, filtering,
            #     round history navigation, and cross-session carry-forward with fake emails ---
            iter_created = json.loads(
                run(
                    "brainstorm", "add",
                    "--id", "iter-e2e",
                    "--title", "Multi-Round Sandbox E2E",
                    "--preset", "Preset: zero-emission shuttle\nPreset: solar bus shelter",
                    "--json",
                    extra={"GIT_AUTHOR_EMAIL": FACILITATOR_EMAIL},
                ).stdout
            )
            iter_key = iter_created["ownerKey"]
            iter_env = {"BRAINSTORM_KEY": iter_key}
            # Admit alice@example.test and bob@example.test via single-use invites.
            for email in (ALICE_EMAIL, BOB_EMAIL):
                inv = post("/api/action", {"action": "invite", "session": "iter-e2e", "name": email}, key=iter_key, email=FACILITATOR_EMAIL)
                post("/api/action", {"action": "join", "session": "iter-e2e", "invite": inv["inviteCode"]}, email=email)

            # During Round 1 write phase, both alice@example.test and bob@example.test see the 2 preset cards.
            alice_w1 = request_json("/api/state?session=iter-e2e", email=ALICE_EMAIL)
            assert alice_w1["round"] == 1 and alice_w1["phase"] == "write" and len(alice_w1["cards"]) == 2
            assert all(c.get("preset") is True for c in alice_w1["cards"])

            # Alice and Bob each submit a private idea during Round 1 write phase.
            post("/api/action", {"action": "add", "session": "iter-e2e", "column": 0, "name": ALICE_EMAIL, "text": "Alice: microgrid battery"}, email=ALICE_EMAIL)
            post("/api/action", {"action": "add", "session": "iter-e2e", "column": 1, "name": BOB_EMAIL, "text": "Bob: paper flyers"}, email=BOB_EMAIL)

            # Bob sees the 2 presets + his own card (3 visible of 4 total), never Alice's unrevealed card.
            bob_w1 = request_json("/api/state?session=iter-e2e", email=BOB_EMAIL)
            assert bob_w1["total"] == 4 and len(bob_w1["cards"]) == 3
            assert not any("Alice: microgrid" in c["text"] for c in bob_w1["cards"])

            # Facilitator reveals and opens voting for Round 1 via CLI.
            run("brainstorm", "set", "iter-e2e", "--phase", "discuss", extra=iter_env)
            run("brainstorm", "set", "iter-e2e", "--phase", "vote", extra=iter_env)
            r1_cards = {c["text"]: c["id"] for c in request_json("/api/state?session=iter-e2e", email=ALICE_EMAIL)["cards"]}
            # Both Alice and Bob vote for "Preset: zero-emission shuttle" and "Alice: microgrid battery" (2 votes each).
            for voter in (ALICE_EMAIL, BOB_EMAIL):
                post("/api/action", {"action": "vote", "session": "iter-e2e", "id": r1_cards["Preset: zero-emission shuttle"]}, email=voter)
                post("/api/action", {"action": "vote", "session": "iter-e2e", "id": r1_cards["Alice: microgrid battery"]}, email=voter)

            # Facilitator advances to Round 2 in "write" phase, filtering by min-votes=2.
            run("brainstorm", "set", "iter-e2e", "--next-round", "--start-phase", "write", "--min-votes", "2", extra=iter_env)
            r2_state = json.loads(run("brainstorm", "show", "iter-e2e", extra=iter_env).stdout)
            assert r2_state["round"] == 2 and r2_state["phase"] == "write" and len(r2_state["cards"]) == 2
            assert all(c.get("priorVotes") == 2 and c.get("votes") == 0 for c in r2_state["cards"])
            assert len(r2_state["rounds"]) == 1 and r2_state["rounds"][0]["advancedCount"] == 2 and r2_state["rounds"][0]["totalCount"] == 4

            # Inspect Round 1 history via CLI (--round 1) and HTTP (?round=1).
            r1_hist_cli = json.loads(run("brainstorm", "show", "iter-e2e", "--round", "1", extra=iter_env).stdout)
            r1_hist_http = request_json("/api/state?session=iter-e2e&round=1", email=BOB_EMAIL)
            assert r1_hist_cli["viewingRound"] == 1 and r1_hist_http["viewingRound"] == 1
            assert sum(1 for c in r1_hist_http["cards"] if c.get("advanced")) == 2
            assert sum(1 for c in r1_hist_http["cards"] if not c.get("advanced")) == 2

            # Add a new card in Round 2 write phase, reveal, vote, and advance to Round 3 skipping directly to vote (top-n=1).
            post("/api/action", {"action": "add", "session": "iter-e2e", "column": 2, "name": BOB_EMAIL, "text": "Bob: community heat pump"}, email=BOB_EMAIL)
            run("brainstorm", "set", "iter-e2e", "--phase", "discuss", extra=iter_env)
            run("brainstorm", "set", "iter-e2e", "--phase", "vote", extra=iter_env)
            r2_cards = {c["text"]: c["id"] for c in request_json("/api/state?session=iter-e2e", email=ALICE_EMAIL)["cards"]}
            for voter in (ALICE_EMAIL, BOB_EMAIL):
                post("/api/action", {"action": "vote", "session": "iter-e2e", "id": r2_cards["Bob: community heat pump"]}, email=voter)
            run("brainstorm", "set", "iter-e2e", "--next-round", "--start-phase", "vote", "--top-n", "1", extra=iter_env)
            r3_state = json.loads(run("brainstorm", "show", "iter-e2e", extra=iter_env).stdout)
            assert r3_state["round"] == 3 and r3_state["phase"] == "vote" and len(r3_state["cards"]) == 1
            assert r3_state["cards"][0]["text"] == "Bob: community heat pump" and r3_state["cards"][0]["priorVotes"] == 2

            # Carry forward Round 1 winners into a new session and verify reopen-round on iter-e2e.
            followup = json.loads(
                run(
                    "brainstorm", "add",
                    "--id", "iter-followup",
                    "--from-session", "iter-e2e",
                    "--from-round", "1",
                    "--from-key", iter_key,
                    "--min-votes", "2",
                    "--start-phase", "vote",
                    "--json",
                ).stdout
            )
            followup_state = json.loads(run("brainstorm", "show", "iter-followup", "--key", followup["ownerKey"]).stdout)
            assert followup_state["fromSession"] == "iter-e2e" and followup_state["fromRound"] == 1
            assert followup_state["phase"] == "vote" and len(followup_state["cards"]) == 2
            run("brainstorm", "set", "iter-e2e", "--reopen-round", "2", extra=iter_env)
            reopened = json.loads(run("brainstorm", "show", "iter-e2e", extra=iter_env).stdout)
            assert reopened["round"] == 2 and len(reopened["cards"]) == 3

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
            print("PASS: isolated sandbox E2E (fake emails, presets, multi-round ideation/voting, filtering, round history, carry-forward, script/command/skill parity)")
        finally:
            try:
                os.killpg(server.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            server.wait(timeout=10)
