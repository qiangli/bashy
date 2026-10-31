"""tutor plan / apply — story bc05f6ea6e8e (Sprint 547)."""
import json
import os
import stat
import unittest
from pathlib import Path

from tests.helpers import TempBashy

import plan


class FormatParseTest(unittest.TestCase):
    """The tiny front-matter parser: `key: value` and `- item` lists only."""

    def test_session_format_fields(self):
        fmt = plan.load_format("session")
        self.assertEqual(fmt["minutes"], 90)
        self.assertTrue(fmt["phases"])
        self.assertEqual([p["name"] for p in fmt["phases"]],
                         ["warm-up", "work", "demo", "retro"])
        self.assertEqual(sum(p["minutes"] for p in fmt["phases"]), 90)
        self.assertGreater(fmt["default_step_minutes"], 0)
        self.assertIn("halfway", fmt["nudge_points"])
        self.assertTrue(fmt["pairing"])
        self.assertTrue(fmt["owner_steps"])
        self.assertTrue(fmt["desk_tools"])
        self.assertTrue(fmt["tone"].strip())
        self.assertIn("guidance", fmt["body"].lower())

    def test_percent_phase_scales(self):
        text = "---\nname: pct\nminutes: 60\nphases:\n- work 50%\n- retro 50%\ndefault_step_minutes: 15\n---\nbody\n"
        fmt = plan.parse_format(text)
        phases = plan.scale_phases(fmt, 40)
        self.assertEqual([p["minutes"] for p in phases], [20, 20])

    def test_timebox_is_a_ceiling_and_sums_exactly(self):
        fmt = plan.load_format("session")
        for minutes in (3, 12, 25, 40, 90, 180):
            participant = [s for s in plan.template_steps(fmt, minutes, ["a", "b"], "g")
                       if s["role"] == "participant"]
            self.assertEqual(sum(s["minutes"] for s in participant), minutes)
            for s in participant:
                self.assertLessEqual(s["minutes"], fmt["default_step_minutes"])
                self.assertGreater(s["minutes"], 0)

    def test_unknown_format_fails(self):
        with self.assertRaises(SystemExit):
            plan.load_format("no-such-format")


class TemplatePlanTest(TempBashy):
    def plan_json(self, *args):
        out = self.tutor("plan", *args).stdout
        return json.loads(out)

    def test_no_agent_template_split(self):
        d = self.plan_json("Ship the parser", "--minutes", "60",
                           "--people", "alice,bob", "--json")
        self.assertEqual(d["version"], 1)
        self.assertEqual(d["source"], "template")
        self.assertEqual(d["minutes"], 60)
        self.assertEqual(d["goal"], "Ship the parser")
        self.assertEqual(d["format"], "session")
        self.assertEqual(d["people"], ["alice", "bob"])
        participant = [s for s in d["steps"] if s["role"] == "participant"]
        owner = [s for s in d["steps"] if s["role"] == "owner"]
        self.assertEqual(sum(s["minutes"] for s in participant), 60)
        self.assertTrue(owner)
        self.assertEqual({s["owner"] for s in owner}, {"owner"})
        self.assertEqual([s["owner"] for s in participant[:4]],
                         ["alice", "bob", "alice", "bob"])
        for s in d["steps"]:
            self.assertTrue(s["title"].strip())
            self.assertTrue(s["done_when"].strip())
            self.assertGreater(s["minutes"], 0)
            self.assertTrue(s["phase"])

    def test_no_people_leaves_owner_empty(self):
        d = self.plan_json("Solo work", "--minutes", "25", "--json")
        participant = [s for s in d["steps"] if s["role"] == "participant"]
        self.assertEqual(sum(s["minutes"] for s in participant), 25)
        self.assertEqual({s["owner"] for s in participant}, {""})
        self.assertEqual(d["people"], [])

    def test_title_and_epic_flags(self):
        d = self.plan_json("Goal text", "--minutes", "30", "--title", "Week 3",
                           "--epic", "session-demo", "--json")
        self.assertEqual(d["title"], "Week 3")
        self.assertEqual(d["epic"], "session-demo")

    def test_draft_validates(self):
        d = self.plan_json("Goal text", "--minutes", "45", "--json")
        self.assertIs(plan.common.validate_draft(d), d)


class StubAgentTest(TempBashy):
    """BASHY_BIN points at a fake that answers `chat` and execs real bashy otherwise."""

    def stub(self, output):
        envelope = {"schema_version": "bashy-chat-v1", "agent": "stub",
                    "exit_code": 0, "output": output}
        out = self.root / "stub-out.json"
        out.write_text(json.dumps(envelope))
        script = self.root / "fake-bashy"
        script.write_text("#!/bin/sh\n"
                          'case "$1" in\n'
                          'chat) cat "%s"; exit 0;;\n'
                          "esac\n"
                          'exec bashy "$@"\n' % out)
        script.chmod(script.stat().st_mode | stat.S_IEXEC)
        os.environ["BASHY_BIN"] = str(script)
        return script

    def test_valid_agent_json_is_used(self):
        draft = {"version": 1, "title": "Agent card", "goal": "Ship it",
                 "epic": "session-demo", "minutes": 30, "format": "session",
                 "people": ["alice"],
                 "steps": [{"title": "Read", "owner": "alice", "minutes": 10,
                            "done_when": "notes taken", "phase": "warm-up",
                            "role": "participant"},
                           {"title": "Build", "owner": "alice", "minutes": 20,
                            "done_when": "tests pass", "phase": "work",
                            "role": "participant"}]}
        self.stub("Here is the plan:\n```json\n%s\n```\nGood luck!" % json.dumps(draft))
        d = json.loads(self.tutor("plan", "Ship it", "--minutes", "30",
                                  "--people", "alice", "--agent", "stub",
                                  "--json").stdout)
        self.assertEqual(d["source"], "agent")
        self.assertEqual(d["title"], "Agent card")
        self.assertEqual([s["title"] for s in d["steps"]], ["Read", "Build"])

    def test_garbage_agent_output_falls_back_to_template(self):
        self.stub("I am afraid I cannot do that.")
        proc = self.tutor("plan", "Ship it", "--minutes", "30",
                          "--agent", "stub", "--json")
        d = json.loads(proc.stdout)
        self.assertEqual(d["source"], "template")
        self.assertEqual(d["minutes"], 30)

    def test_invalid_agent_draft_falls_back_to_template(self):
        self.stub(json.dumps({"version": 1, "title": "", "goal": "g",
                              "minutes": 0, "steps": []}))
        d = json.loads(self.tutor("plan", "Ship it", "--minutes", "30",
                                  "--agent", "stub", "--json").stdout)
        self.assertEqual(d["source"], "template")


class ApplyTest(TempBashy):
    def test_apply_creates_card_people_and_labelled_todos(self):
        draft = json.loads(self.tutor("plan", "Ship the parser", "--minutes", "60",
                                      "--people", "alice,bob", "--json").stdout)
        res = json.loads(self.tutor("apply", "-", "--json",
                                    stdin=json.dumps(draft)).stdout)
        self.assertIsInstance(res["sprint"], int)
        self.assertEqual(len(res["steps"]), len(draft["steps"]))
        self.assertEqual([s["index"] for s in res["steps"]],
                         list(range(1, len(draft["steps"]) + 1)))
        for s in res["steps"]:
            self.assertTrue(s["id"])

        nodes = self.todo_nodes("step-01")
        self.assertEqual(len(nodes), 1)
        attrs = nodes[0]["attrs"]
        self.assertEqual(attrs["title"], draft["steps"][0]["title"])
        self.assertEqual(attrs["assignee"], draft["steps"][0]["owner"])
        self.assertIn("role-participant", attrs["labels"])
        self.assertIn("timebox-%dm" % draft["steps"][0]["minutes"], attrs["labels"])
        self.assertEqual(attrs["sprint"], res["sprint"])
        self.assertIn("phase-%s" % draft["steps"][0]["phase"].replace("-", ""),
                      attrs["labels"])

        owner = [s for s in draft["steps"] if s["role"] == "owner"][0]
        idx = draft["steps"].index(owner) + 1
        tnodes = self.todo_nodes("step-%02d" % idx)
        self.assertEqual(tnodes[0]["attrs"]["assignee"], "owner")

    def test_personal_steps_and_completion_default(self):
        draft = json.loads(self.tutor("plan", "Example", "--people", "alice", "--json").stdout)
        draft["default_done_when"] = "Show a working example"
        draft["steps"][0]["done_when"] = ""
        result = json.loads(self.tutor("apply", "-", "--json", stdin=json.dumps(draft)).stdout)
        value = json.loads(self.tutor("status", str(result["sprint"]), "--json").stdout)
        self.assertEqual(value["steps"][0]["done_when"], "Show a working example")
        from common import bashy
        from run import records
        self.assertEqual(records(bashy("todo", "list", "--all", "--json"), "todo"), [])
        self.assertFalse((self.repo / "docs" / "todo").exists())

    def test_apply_refuses_invalid_draft_before_creating_anything(self):
        bad = {"version": 1, "title": "t", "goal": "g", "minutes": 10, "steps": []}
        proc = self.tutor("apply", "-", "--json", stdin=json.dumps(bad), check=False)
        self.assertNotEqual(proc.returncode, 0)
        self.assertIn("steps", proc.stderr)
        self.assertEqual(self.todo_nodes(), [])

    def test_apply_reads_a_file(self):
        draft = json.loads(self.tutor("plan", "Read a file", "--minutes", "20",
                                      "--people", "alice", "--json").stdout)
        path = self.root / "draft.json"
        path.write_text(json.dumps(draft))
        res = json.loads(self.tutor("apply", str(path), "--json").stdout)
        self.assertEqual(len(res["steps"]), len(draft["steps"]))

    def todo_nodes(self, label=None):
        args = ["todo", "list", "--user", "--json"]
        if label:
            args += ["--label", label]
        import subprocess
        proc = subprocess.run(["bashy", *args], text=True, capture_output=True,
                              cwd=str(self.repo))
        nodes = []
        for line in proc.stdout.splitlines():
            line = line.strip()
            if not line:
                continue
            rec = json.loads(line)
            if rec.get("t") == "node":
                nodes.append(rec)
        return nodes


if __name__ == "__main__":
    unittest.main()
