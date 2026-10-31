"""Tests for tutor render (Sprint 547, Story 9d0a95cf258d)."""
import http.server
import json
import os
import subprocess
import threading
from pathlib import Path

from tests.helpers import TempBashy

HERE = Path(__file__).resolve().parent.parent


class RenderTest(TempBashy):
    def setUp(self):
        super().setUp()
        # Register bashy form command in temporary BASHY_HOME
        form_bsh = HERE.parent / "desk" / "form" / "form.bsh"
        if form_bsh.is_file():
            subprocess.run([
                "bashy", "commands", "add", "form", "--force",
                "--set", "exec.0=bashy", "--set", "exec.1=--bashsharp",
                "--set", f"exec.2={form_bsh}"
            ], capture_output=True, text=True)

    def test_participant_edition_contains_no_owner_content(self):
        secret_marker = "OWNER_SECRET_MARKER_9d0a95cf"
        secret_block = "OWNER_BLOCK_MARKER_abcdef123"
        step_content = f"""---
title: Sample Lesson
done_when: all tasks done
questions:
  - id: q1
    label: What is 2 + 2?
    kind: text
    answer: {secret_marker}
    answer_rubric: Exact match only
answer_notes: Top secret owner notes
---
# Lesson One

Welcome participants. Please answer the questions.

~~~private
{secret_block}
Full owner guide and solution key.
~~~

Follow-up exercise instructions.
"""
        step_file = self.repo / "content" / "step-01.md"
        step_file.parent.mkdir(parents=True, exist_ok=True)
        step_file.write_text(step_content, encoding="utf-8")

        # Test markdown format
        res = self.tutor("render", str(step_file), "--edition", "participant")
        out = res.stdout
        self.assertNotIn(secret_marker, out)
        self.assertNotIn(secret_block, out)
        self.assertNotIn("~~~private", out)
        self.assertNotIn("answer_notes", out)
        self.assertNotIn("answer_rubric", out)
        self.assertIn("Welcome participants.", out)
        self.assertIn("What is 2 + 2?", out)

        # Test JSON format
        res_json = self.tutor("render", str(step_file), "--edition", "participant", "--format", "json")
        data = json.loads(res_json.stdout)
        self.assertEqual(data["edition"], "participant")
        self.assertNotIn(secret_marker, res_json.stdout)
        self.assertNotIn(secret_block, res_json.stdout)
        self.assertTrue(len(data.get("questions", [])) > 0)
        for q in data["questions"]:
            for k in q:
                self.assertFalse(k.lower().startswith("answer"), f"Leaked {k} in participant question")

    def test_owner_edition_contains_owner_content(self):
        secret_marker = "OWNER_SECRET_MARKER_9d0a95cf"
        secret_block = "OWNER_BLOCK_MARKER_abcdef123"
        step_content = f"""---
title: Sample Lesson
done_when: all tasks done
questions:
  - id: q1
    label: What is 2 + 2?
    kind: text
    answer: {secret_marker}
---
# Lesson One

Welcome participants.

~~~private
{secret_block}
Full owner guide.
~~~
"""
        step_file = self.repo / "content" / "step-01.md"
        step_file.parent.mkdir(parents=True, exist_ok=True)
        step_file.write_text(step_content, encoding="utf-8")

        # Test markdown format
        res = self.tutor("render", str(step_file), "--edition", "owner")
        out = res.stdout
        self.assertIn(secret_marker, out)
        self.assertIn(secret_block, out)
        self.assertIn("> Private:", out)

        # Test JSON format
        res_json = self.tutor("render", str(step_file), "--edition", "owner", "--format", "json")
        data = json.loads(res_json.stdout)
        self.assertEqual(data["edition"], "owner")
        self.assertIn(secret_marker, res_json.stdout)
        self.assertIn(secret_block, res_json.stdout)
        self.assertEqual(data["questions"][0]["answer"], secret_marker)

    def test_identical_editions_when_no_blocks(self):
        step_content = """---
title: Neutral Lesson
done_when: completed
questions:
  - id: q1
    label: What is your favorite language?
    kind: text
---
# Neutral Lesson

Just regular content without any owner blocks or answer keys.
"""
        step_file = self.repo / "content" / "step-neutral.md"
        step_file.parent.mkdir(parents=True, exist_ok=True)
        step_file.write_text(step_content, encoding="utf-8")

        res_participant = self.tutor("render", str(step_file), "--edition", "participant")
        res_owner = self.tutor("render", str(step_file), "--edition", "owner")
        self.assertEqual(res_participant.stdout, res_owner.stdout)

    def test_form_def_round_trip(self):
        step_content = """---
title: Test Questionnaire
done_when: submitted
questions:
  - id: q1
    label: What is your name?
    kind: text
    required: true
    answer: Participant Name
  - id: q2
    label: Select an option
    kind: choice
    options: ["Option A", "Option B"]
    recommended: Option A
    answer: Option A
---
# Questionnaire Step
"""
        step_file = self.repo / "content" / "step-form.md"
        step_file.parent.mkdir(parents=True, exist_ok=True)
        step_file.write_text(step_content, encoding="utf-8")

        # Start a fake form server on loopback
        received_requests = []

        class FakeFormHandler(http.server.BaseHTTPRequestHandler):
            def do_POST(self):
                length = int(self.headers.get("content-length", 0))
                body = self.rfile.read(length)
                received_requests.append(json.loads(body.decode("utf-8")))
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                resp = {
                    "id": "form-fake-123",
                    "status": "open",
                    "url": "http://127.0.0.1/f/form-fake-123",
                    "key": "owner-secret-key"
                }
                self.wfile.write(json.dumps(resp).encode("utf-8"))

            def log_message(self, *args):
                pass

        server = http.server.HTTPServer(("127.0.0.1", 0), FakeFormHandler)
        try:
            port = server.server_address[1]
            t = threading.Thread(target=server.handle_request, daemon=True)
            t.start()

            env = dict(os.environ, FORM_ADDR=f"127.0.0.1:{port}", BASHY_HINTS="off")

            # Render form definition
            render_res = self.tutor("render", str(step_file), "--edition", "participant", "--format", "form")
            form_def_str = render_res.stdout
            form_def = json.loads(form_def_str)
            self.assertEqual(form_def["title"], "Test Questionnaire")
            self.assertEqual(len(form_def["fields"]), 2)

            # Pipe into bashy form add --def -
            proc = subprocess.run(
                ["bashy", "form", "add", "--def", "-"],
                input=form_def_str,
                text=True,
                capture_output=True,
                env=env
            )
            self.assertEqual(proc.returncode, 0, f"form add failed: {proc.stderr}")
            self.assertEqual(len(received_requests), 1)
            req = received_requests[0]
            self.assertEqual(req["action"], "add")
            self.assertEqual(req["title"], "Test Questionnaire")
            self.assertEqual(len(req["definition"]), 2)
            self.assertEqual(req["definition"][0]["id"], "q1")
            self.assertEqual(req["definition"][1]["id"], "q2")
        finally:
            server.server_close()

    def test_example_step_file(self):
        example_step = HERE / "content" / "example-step.md"
        self.assertTrue(example_step.is_file(), "content/example-step.md must exist")

        res_stud = self.tutor("render", "example-step", "--edition", "participant")
        self.assertNotIn("~~~private", res_stud.stdout)

        res_teach = self.tutor("render", "example-step", "--edition", "owner")
        self.assertIn("> Private:", res_teach.stdout)


if __name__ == "__main__":
    import unittest
    unittest.main()
