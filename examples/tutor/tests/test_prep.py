"""Tests for tutor prep (Sprint 547, Story f8feda454ff8)."""
import json
import os
import re
import shutil
import unittest
from pathlib import Path

from tests.helpers import TempBashy

import common
import plan


class PrepTest(TempBashy):
    def todo_nodes(self, sprint=None, label=None):
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
            try:
                rec = json.loads(line)
                if rec.get("t") == "node":
                    if sprint is None or rec.get("attrs", {}).get("sprint") == sprint:
                        nodes.append(rec)
            except Exception:
                pass
        return nodes

    def test_prep_from_scratch_no_agent(self):
        target = self.repo / "session1"
        res = self.tutor("prep", str(target), "--goal", "Learn Bashy Basics",
                         "--minutes", "60", "--json")
        data = json.loads(res.stdout)
        self.assertEqual(data["goal"], "Learn Bashy Basics")
        self.assertEqual(data["minutes"], 60)
        self.assertTrue(target.is_dir())

        # format.md must exist and match session format
        fmt_file = target / "format.md"
        self.assertTrue(fmt_file.is_file())
        self.assertIn("name: session", fmt_file.read_text())

        # draft.json must exist and validate
        draft_file = target / "draft.json"
        self.assertTrue(draft_file.is_file())
        draft = json.loads(draft_file.read_text())
        common.validate_draft(draft)
        self.assertEqual(draft["goal"], "Learn Bashy Basics")
        self.assertEqual(draft["minutes"], 60)

        # content/ directory must exist and have files with empty ~~~private blocks
        content_dir = target / "content"
        self.assertTrue(content_dir.is_dir())
        md_files = sorted(content_dir.glob("*.md"))
        self.assertTrue(len(md_files) > 0)

        participant_steps = [s for s in draft["steps"] if s["role"] == "participant"]
        owner_steps = [s for s in draft["steps"] if s["role"] == "owner"]
        self.assertEqual(len(md_files), len(participant_steps))
        self.assertTrue(len(owner_steps) >= 3)  # prepare, facilitate, review

        for i, s in enumerate(participant_steps):
            expected_content = f"content/{md_files[i].name}"
            self.assertEqual(s["content"], expected_content)
            # Check NN-slug pattern
            self.assertTrue(re.match(r"^\d\d-[a-z0-9-]+\.md$", md_files[i].name))
            text = md_files[i].read_text()
            self.assertIn("~~~private", text)
            self.assertIn("~~~", text)

        # Check non-json invocation
        target2 = self.repo / "session2"
        res2 = self.tutor("prep", str(target2), "--goal", "Second Session", "--minutes", "30")
        self.assertIn("prepared", res2.stdout.lower())
        self.assertTrue((target2 / "draft.json").is_file())

    def test_prep_from_fixture_copy_modify_and_apply(self):
        # 1. Create fixture directory
        fixture = self.repo / "fixture_dir"
        fixture.mkdir()
        shutil.copy(plan.FORMATS / "session.md", fixture / "format.md")
        content_dir = fixture / "content"
        content_dir.mkdir()

        step1 = content_dir / "01-warm-up.md"
        step1.write_text("""---
title: Warm-up Exercise
timebox: 10m
role: participant
phase: warm-up
done_when: completed warmup notes
---
# Warm-up Exercise

Welcome to the session!

~~~private
~~~
""")

        step2 = content_dir / "02-hands-on.md"
        step2.write_text("""---
title: Hands-on Lab
timebox: 25m
role: participant
phase: work
done_when: all tests pass
---
# Hands-on Lab

Work on the lab.

~~~private
Check test results
~~~
""")

        # 2. Prep from fixture
        target = self.repo / "session-copy"
        res = self.tutor("prep", str(target), "--from", str(fixture),
                         "--goal", "Advanced Git & Bashy", "--json")
        data = json.loads(res.stdout)
        self.assertEqual(data["goal"], "Advanced Git & Bashy")

        draft = json.loads((target / "draft.json").read_text())
        common.validate_draft(draft)
        participant_steps = [s for s in draft["steps"] if s["role"] == "participant"]
        owner_steps = [s for s in draft["steps"] if s["role"] == "owner"]
        self.assertEqual(len(participant_steps), 2)
        self.assertEqual(participant_steps[0]["title"], "Warm-up Exercise")
        self.assertEqual(participant_steps[0]["content"], "content/01-warm-up.md")
        self.assertEqual(participant_steps[1]["title"], "Hands-on Lab")
        self.assertEqual(participant_steps[1]["content"], "content/02-hands-on.md")
        self.assertEqual([s["phase"] for s in owner_steps],
                         ["prepare", "facilitate", "review"])

        # 3. Modify content in target
        (target / "content" / "02-hands-on.md").write_text("""---
title: Hands-on Lab Extended
timebox: 35m
role: participant
phase: work
done_when: all tests and lint pass
---
# Hands-on Lab Extended
~~~private
~~~
""")
        # Add another content file
        (target / "content" / "03-demo.md").write_text("""---
title: Peer Demo
timebox: 15m
role: participant
phase: demo
done_when: demonstrated to peer
---
# Peer Demo
~~~private
~~~
""")

        # Refresh by running prep on the target dir
        self.tutor("prep", str(target), "--json")
        draft = json.loads((target / "draft.json").read_text())
        participant_steps = [s for s in draft["steps"] if s["role"] == "participant"]
        self.assertEqual(len(participant_steps), 3)
        self.assertEqual(participant_steps[1]["title"], "Hands-on Lab Extended")
        self.assertEqual(participant_steps[1]["minutes"], 35)
        self.assertEqual(participant_steps[1]["done_when"], "all tests and lint pass")
        self.assertEqual(participant_steps[2]["title"], "Peer Demo")
        self.assertEqual(participant_steps[2]["content"], "content/03-demo.md")

        # 4. Apply the prepped draft
        apply_res = self.tutor("apply", str(target / "draft.json"), "--json")
        apply_data = json.loads(apply_res.stdout)
        sprint_num = apply_data["sprint"]
        self.assertIsInstance(sprint_num, int)

        # 5. Verify stories produced on sprint card
        nodes = self.todo_nodes(sprint=sprint_num)
        self.assertEqual(len(nodes), len(draft["steps"]))

        # Check participant stories
        participant_nodes = [n for n in nodes if "role-participant" in n["attrs"]["labels"]]
        owner_nodes = [n for n in nodes if "role-owner" in n["attrs"]["labels"]]
        self.assertEqual(len(participant_nodes), 3)
        self.assertEqual(len(owner_nodes), 3)

        titles = [n["attrs"]["title"] for n in participant_nodes]
        self.assertIn("Warm-up Exercise", titles)
        self.assertIn("Hands-on Lab Extended", titles)
        self.assertIn("Peer Demo", titles)

        owner_titles = [n["attrs"]["title"] for n in owner_nodes]
        self.assertTrue(any("Prepare" in t for t in owner_titles))
        self.assertTrue(any("Facilitate" in t for t in owner_titles))
        self.assertTrue(any("Review" in t for t in owner_titles))

    def test_prep_missing_from_fails(self):
        proc = self.tutor("prep", "invalid", "--from", "non_existent_directory_xyz",
                          check=False)
        self.assertNotEqual(proc.returncode, 0)
        self.assertIn("not found", proc.stderr)

    def test_prep_target_is_file_fails(self):
        file_target = self.repo / "some_file.txt"
        file_target.write_text("hello")
        proc = self.tutor("prep", str(file_target), check=False)
        self.assertNotEqual(proc.returncode, 0)
        self.assertIn("not a directory", proc.stderr)

    def test_prep_render_compatibility(self):
        target = self.repo / "session_render"
        self.tutor("prep", str(target), "--goal", "Render Test", "--minutes", "30", "--json")
        step_file = target / "content" / "01-warm-up-render-test.md"
        if not step_file.is_file():
            step_file = sorted((target / "content").glob("*.md"))[0]

        # Participant edition hides owner notes
        res_participant = self.tutor("render", str(step_file), "--edition", "participant")
        self.assertNotIn("~~~private", res_participant.stdout)
        self.assertNotIn("> Private:", res_participant.stdout)

        # Owner edition renders owner block
        res_owner = self.tutor("render", str(step_file), "--edition", "owner")
        self.assertIn("> Private:", res_owner.stdout)

    def test_prep_with_agent(self):
        import stat
        draft = {
            "version": 1,
            "title": "Agent Prepped Session",
            "goal": "Build Agent Tools",
            "epic": "agent-tools",
            "minutes": 40,
            "format": "session",
            "people": ["alice", "bob"],
            "steps": [
                {
                    "title": "Agent Discovery Step",
                    "owner": "alice",
                    "minutes": 15,
                    "done_when": "discovered tools",
                    "phase": "warm-up",
                    "role": "participant"
                },
                {
                    "title": "Agent Implementation Step",
                    "owner": "bob",
                    "minutes": 25,
                    "done_when": "tools implemented",
                    "phase": "work",
                    "role": "participant"
                }
            ]
        }
        envelope = {
            "schema_version": "bashy-chat-v1",
            "agent": "agent-x",
            "exit_code": 0,
            "output": f"```json\n{json.dumps(draft)}\n```"
        }
        out_file = self.root / "stub-out.json"
        out_file.write_text(json.dumps(envelope))
        script = self.root / "fake-bashy"
        script.write_text(f'#!/bin/sh\ncase "$1" in\nchat) cat "{out_file}"; exit 0;;\nesac\nexec bashy "$@"\n')
        script.chmod(script.stat().st_mode | stat.S_IEXEC)
        os.environ["BASHY_BIN"] = str(script)

        target = self.repo / "agent_session"
        res = self.tutor("prep", str(target), "--goal", "Build Agent Tools",
                         "--agent", "agent-x", "--json")
        data = json.loads(res.stdout)
        self.assertEqual(data["title"], "Agent Prepped Session")
        self.assertEqual(data["goal"], "Build Agent Tools")
        self.assertEqual(len(data["content_files"]), 2)
        self.assertEqual(data["content_files"][0], "content/01-agent-discovery-step.md")
        self.assertEqual(data["content_files"][1], "content/02-agent-implementation-step.md")

        draft_loaded = json.loads((target / "draft.json").read_text())
        participant_steps = [s for s in draft_loaded["steps"] if s["role"] == "participant"]
        owner_steps = [s for s in draft_loaded["steps"] if s["role"] == "owner"]
        self.assertEqual(len(participant_steps), 2)
        self.assertTrue(len(owner_steps) >= 3)



if __name__ == "__main__":
    unittest.main()
