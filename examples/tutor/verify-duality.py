#!/usr/bin/env python3
"""Real registration and Nov 1 rehearsal; every mutable store is temporary.

The stub only supplies a chat draft and acknowledges sprint start. It never
launches an agent. This proves tutor's boundary, not a managed agent session.
"""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess

from tests.helpers import HERE, TempBashy


def verify(mode):
    fixture = TempBashy()
    fixture.setUp()
    try:
        root = fixture.root
        real = shutil.which(os.environ.get("BASHY_BIN", "bashy"))
        assert real, "bashy must be installed"
        os.environ.update(BASHY_BIN=real, BASHY_APPS_DIR=str(root / "apps"),
                          BASHY_APPS_PATH="", XDG_CONFIG_HOME=str(root / "config"),
                          XDG_CACHE_HOME=str(root / "cache"),
                          TUTOR_NOW="2026-11-01T18:00:00Z")
        def call(*args, ok=True):
            p = subprocess.run([real, *map(str, args)], capture_output=True,
                               text=True, timeout=60)
            assert (p.returncode == 0) == ok, (args, p.stdout, p.stderr)
            return p.stdout if ok else p.stderr

        def tutor(*args):
            return call("tutor", *args)

        call(HERE / "register.sh")
        guide = call("--bashsharp", HERE / "tutor.bsh", "--skill")
        assert guide.strip() == call("skill", "show", "tutor").strip()
        atlas = call("commands", "show", "tutor", "--json")
        assert "tutor.bsh" in atlas
        assert list((root / "apps").glob("*")), "app must be in isolated registry"
        goal = "Week 2: Klatch sign-up, groups and after-class use"
        draft = json.loads(tutor("plan", goal, "--minutes", "90", "--people", "ana,ben", "--json"))
        assert draft["source"] == "template"
        if mode == "stub":
            # A deterministic response at the chat boundary; no provider credentials.
            response = root / "response.json"
            response.write_text(json.dumps({"output": json.dumps(draft)}))
            stub = root / "stub-bashy"
            stub.write_text("#!/usr/bin/env python3\n"
                            "import json, os, sys\n"
                            f"real = {real!r}\n"
                            "args = sys.argv[1:]\n"
                            "if args[:1] == ['chat']:\n"
                            f"    print(open({str(response)!r}).read())\n"
                            "elif args[:2] == ['sprint', 'start']:\n"
                            "    print('{}')\n"
                            "else:\n"
                            "    os.execv(real, [real, *args])\n")
            stub.chmod(0o700)
            os.environ["BASHY_BIN"] = str(stub)
            draft = json.loads(tutor("plan", goal, "--agent", "stub", "--minutes", "90", "--json"))
            assert draft["source"] == "agent"

        # Review the generated draft into a concrete, five-step Week 2 rehearsal.
        # Timings are rehearsal choices, not a claim about the live class plan.
        agenda = [
            ("Sign in to Klatch", 15, "Sign-in confirmed", "warmup"),
            ("Join the class group", 20, "Greeting visible in the group", "work"),
            ("Try a DM and @mention", 25, "Partner receives both messages", "work"),
            ("Demo an after-class question", 20, "Question and reply shown", "demo"),
            ("Reflect and plan after-class use", 10, "Next use noted", "retro"),
        ]
        teachers = [s for s in draft["steps"] if s["role"] == "teacher"]
        draft["steps"] = [
            dict(title=title, minutes=minutes, done_when=done, phase=phase,
                 role="student", owner=["ana", "ben"][i % 2])
            for i, (title, minutes, done, phase) in enumerate(agenda)
        ] + teachers
        path = root / "nov-1.json"
        path.write_text(json.dumps(draft))
        result = json.loads(tutor("apply", path, "--json"))
        sprint = str(result["sprint"])
        assert sprint == "1", "must use an empty temporary board"
        print(f"{mode}: plan source={draft['source']}; apply sprint=1, 5 student + {len(teachers)} teacher steps")
        if mode == "none":
            error = call("tutor", "start", sprint, "--for", "90m", "--json", ok=False)
            assert "--owner" in error and "Traceback" not in error
            print("none: start refused cleanly: explicit manager --owner required; continuing offline with next")
            tutor("next", sprint, "--json")
        else:
            started = json.loads(tutor("start", sprint, "--owner", "stub", "--for", "90m", "--json"))
            assert started["clock"]["state"] == "running"
            assert started["clock"]["remaining_s"] == 5400
            print("stub: start acknowledged by stub; fallback session clock=5400s")
        tutor("done", sprint, result["steps"][0]["id"], "-m", "Rehearsal sign-in evidence", "--json")
        tutor("next", sprint, "--json")
        line = tutor("status", sprint, "--line").strip()
        assert line == "Step 2/5 · Join the class group · 20:00 left · ben", line
        print(f"{mode}: {line}")
        direct = json.loads(call("--bashsharp", HERE / "tutor.bsh", "status", sprint, "--json"))
        registered = json.loads(tutor("status", sprint, "--json"))
        assert direct == registered and len(direct["steps"]) == 8
        assert direct["steps"][0]["evidence"] == "Rehearsal sign-in evidence"
        hint = json.loads(tutor("nudge", sprint, "--person", "ben", "--json"))
        assert hint["done_when"] == "Greeting visible in the group"
        os.environ["TUTOR_SPRINT"] = sprint
        receipt = json.loads(call("skill", "run", "tutor", "--json"))
        assert receipt["valid"] is True, receipt
        os.environ["TUTOR_SPRINT"] = "999"
        call("skill", "run", "tutor", "--json", ok=False)
        print(f"PASS {mode}: script/command parity, skill projection + attestation, missing sprint rejected; live stores untouched")
    finally:
        fixture.tearDown()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=("none", "stub", "both"), default="both")
    args = parser.parse_args()
    for mode in ("none", "stub") if args.mode == "both" else (args.mode,):
        verify(mode)
