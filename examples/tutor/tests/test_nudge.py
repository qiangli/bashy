"""Tests for tutor nudge — guardrailed hints (story 23cede21adca)."""
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

from tests.helpers import TempBashy

HERE = Path(__file__).resolve().parent.parent  # examples/tutor


def _real_bashy():
    # Resolve real bashy binary outside any BASHY_BIN fake
    p = subprocess.run(["which", "bashy"], capture_output=True, text=True)
    if p.returncode == 0 and p.stdout.strip():
        return p.stdout.strip()
    return "bashy"


def _make_fake(capture_file, canned_output, real_bashy):
    """Create a BASHY_BIN fake script that captures chat -m prompt and returns canned output."""
    fd, path = tempfile.mkstemp(prefix="fake-bashy-", suffix=".sh")
    os.write(fd, f"""#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "chat" ]]; then
  # capture the -m / --instruction prompt
  prompt=""
  prev=""
  for arg in "$@"; do
    if [[ "$prev" == "-m" || "$prev" == "--instruction" ]]; then
      prompt="$arg"
    fi
    prev="$arg"
  done
  # also handle --instruction=VALUE and -m=VALUE not used but be safe
  if [[ -z "$prompt" ]]; then
    for arg in "$@"; do
      if [[ "$arg" == --instruction=* ]]; then
        prompt="${{arg#--instruction=}}"
      fi
    done
  fi
  printf "%s" "$prompt" > "{capture_file}"
  cat <<'FAKEEOF'
{canned_output}
FAKEEOF
  exit 0
else
  exec "{real_bashy}" "$@"
fi
""".encode())
    os.close(fd)
    os.chmod(path, 0o755)
    return path


class NudgeTest(TempBashy):
    def setUp(self):
        super().setUp()
        # persons needed for tutor steps (idempotent)
        for p in ("alice", "bob"):
            proc = subprocess.run(["bashy", "person", "add", p], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            if proc.returncode != 0 and "already exists" not in proc.stderr:
                self.fail(f"person add {p} failed: {proc.stderr}")
        # create sprint 1
        subprocess.run(["bashy", "sprint", "add", "Test Sprint", "--json"], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.sprint = 1
        self.real_bashy = _real_bashy()

    def _add_step(self, title, owner, step_label, done_when="", timebox="10m", role="participant"):
        args = ["bashy", "todo", "add", "--user", title, "--sprint", str(self.sprint), "--owner", owner, "--label", step_label, "--label", f"timebox-{timebox}", "--label", f"role-{role}", "--json"]
        if done_when:
            args.extend(["--note", done_when])
        proc = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        if proc.returncode != 0:
            self.fail(f"todo add failed: {proc.stderr} {proc.stdout}")
        data = json.loads(proc.stdout)
        return data["id"]

    def _start(self, todo_id):
        proc = subprocess.run(["bashy", "todo", "start", "--user", todo_id], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        if proc.returncode != 0:
            self.fail(f"todo start failed: {proc.stderr}")

    def test_write_it_for_me_filtered(self):
        """Agent returning code fences / long solution must be replaced with a hint/question."""
        title = "Write it for me: implement fibonacci"
        done_when = "Done when you have written tests for fibonacci"
        tid1 = self._add_step(title, "alice", "step-01", done_when=done_when)
        tid2 = self._add_step("Other participant secret step", "bob", "step-02", done_when="owner edition answer is 42")
        self._start(tid1)
        self._start(tid2)

        capture = tempfile.mktemp(prefix="nudge-capture-")
        # canned agent output that looks like a deliverable
        canned = json.dumps({"schema_version": "bashy-chat-v1", "output": "Here is the solution:\n```python\ndef fibonacci(n):\n    return n\n```\nThis is the full answer with code."})
        fake = _make_fake(capture, canned, self.real_bashy)
        env = dict(os.environ, BASHY_BIN=fake, TUTOR_AGENT="fake-agent")
        # also set capture env? fake uses fixed path
        proc = self.tutor("nudge", str(self.sprint), "--person", "alice", "--agent", "fake-agent", "--json", check=False)
        # need to run with env BASHY_BIN? TempBashy.tutor does not pass custom env; it uses os.environ
        # So we patch os.environ before calling tutor
        old = dict(os.environ)
        try:
            os.environ["BASHY_BIN"] = fake
            os.environ["TUTOR_AGENT"] = "fake-agent"
            proc = self.tutor("nudge", str(self.sprint), "--person", "alice", "--agent", "fake-agent", "--json", check=False)
        finally:
            os.environ.clear()
            os.environ.update(old)
        os.unlink(fake)
        self.assertEqual(proc.returncode, 0, f"nudge failed: {proc.stderr} {proc.stdout}")
        data = json.loads(proc.stdout)
        hint = data.get("hint") or data.get("output") or data.get("nudge") or ""
        # must NOT contain code fences or code
        self.assertNotIn("```", hint, "nudge should filter code fences")
        self.assertNotIn("def fibonacci", hint)
        # must be a hint/question (contains ? or fallback phrase)
        self.assertTrue("?" in hint or len(hint.split()) < 80, f"hint should be question-like, got {hint!r}")
        self.assertLessEqual(len(hint.split()), 80)
        # prompt sent should contain title+done_when only, not other participant's data
        prompt = Path(capture).read_text(encoding="utf-8") if Path(capture).exists() else ""
        if Path(capture).exists():
            Path(capture).unlink()
        self.assertIn(title, prompt)
        self.assertIn(done_when, prompt)
        self.assertNotIn("Other participant secret step", prompt)
        self.assertNotIn("owner edition", prompt)

    def test_good_hint_passthrough(self):
        title = "Explore loops"
        done_when = "Done when you can explain the loop condition"
        tid = self._add_step(title, "alice", "step-01", done_when=done_when)
        self._start(tid)
        capture = tempfile.mktemp(prefix="nudge-capture-")
        good_hint = "Have you considered what the loop condition should be when n is 0?"
        canned = json.dumps({"schema_version": "bashy-chat-v1", "output": good_hint})
        fake = _make_fake(capture, canned, self.real_bashy)
        old = dict(os.environ)
        try:
            os.environ["BASHY_BIN"] = fake
            os.environ["TUTOR_AGENT"] = "fake-agent"
            proc = self.tutor("nudge", str(self.sprint), "--person", "alice", "--agent", "fake-agent", "--json", check=False)
        finally:
            os.environ.clear()
            os.environ.update(old)
        os.unlink(fake)
        if Path(capture).exists():
            Path(capture).unlink()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        data = json.loads(proc.stdout)
        hint = data.get("hint") or data.get("output") or ""
        # good hint should be passed through verbatim (or trimmed)
        self.assertEqual(hint.strip(), good_hint)

    def test_no_agent_fallback(self):
        title = "Understand variables"
        done_when = "Done when you can describe what a variable holds"
        tid = self._add_step(title, "alice", "step-01", done_when=done_when)
        self._start(tid)
        # ensure no agent configured
        old = dict(os.environ)
        try:
            os.environ.pop("TUTOR_AGENT", None)
            os.environ.pop("BASHY_BIN", None)  # use real bashy
            # ensure BASHY_BIN not set to fake; nudge should use fallback
            # need to ensure BASHY_BIN is not set to something invalid
            if "BASHY_BIN" in os.environ:
                del os.environ["BASHY_BIN"]
            proc = self.tutor("nudge", str(self.sprint), "--person", "alice", "--json", check=False)
        finally:
            os.environ.clear()
            os.environ.update(old)
        self.assertEqual(proc.returncode, 0, f"nudge without agent should not crash: {proc.stderr}")
        data = json.loads(proc.stdout)
        hint = data.get("hint") or data.get("output") or data.get("nudge") or proc.stdout
        self.assertTrue(hint.strip(), "fallback hint should be non-empty")
        self.assertIn("?", hint, "fallback should be a question")
        # also test plain text mode
        try:
            os.environ.pop("TUTOR_AGENT", None)
            if "BASHY_BIN" in os.environ:
                del os.environ["BASHY_BIN"]
            proc2 = self.tutor("nudge", str(self.sprint), "--person", "alice", check=False)
        finally:
            os.environ.clear()
            os.environ.update(old)
        self.assertEqual(proc2.returncode, 0)
        self.assertIn("?", proc2.stdout)

    def test_first_doing_step_without_person(self):
        # two doing steps, without --person should pick step-01
        tid1 = self._add_step("First step", "alice", "step-01", done_when="first done")
        tid2 = self._add_step("Second step", "bob", "step-02", done_when="second done")
        self._start(tid1)
        self._start(tid2)
        old = dict(os.environ)
        try:
            os.environ.pop("TUTOR_AGENT", None)
            if "BASHY_BIN" in os.environ:
                del os.environ["BASHY_BIN"]
            proc = self.tutor("nudge", str(self.sprint), "--json", check=False)
        finally:
            os.environ.clear()
            os.environ.update(old)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        data = json.loads(proc.stdout)
        self.assertEqual(data.get("step"), "First step")

    def test_long_output_filtered(self):
        title = "Write it for me again"
        done_when = "Done when done"
        tid = self._add_step(title, "alice", "step-01", done_when=done_when)
        self._start(tid)
        capture = tempfile.mktemp(prefix="nudge-capture-")
        long_text = " ".join(["word"] * 100)  # >80 words
        canned = json.dumps({"schema_version": "bashy-chat-v1", "output": long_text})
        fake = _make_fake(capture, canned, self.real_bashy)
        old = dict(os.environ)
        try:
            os.environ["BASHY_BIN"] = fake
            os.environ["TUTOR_AGENT"] = "fake-agent"
            proc = self.tutor("nudge", str(self.sprint), "--person", "alice", "--agent", "fake-agent", "--json", check=False)
        finally:
            os.environ.clear()
            os.environ.update(old)
        os.unlink(fake)
        if Path(capture).exists():
            Path(capture).unlink()
        data = json.loads(proc.stdout)
        hint = data.get("hint") or ""
        self.assertLessEqual(len(hint.split()), 80)
        self.assertNotEqual(hint.strip(), long_text.strip())

    def test_env_agent_resolution(self):
        title = "Env agent test"
        done_when = "Done when env works"
        tid = self._add_step(title, "alice", "step-01", done_when=done_when)
        self._start(tid)
        capture = tempfile.mktemp(prefix="nudge-capture-")
        good = "What small step could you try next?"
        canned = json.dumps({"schema_version": "bashy-chat-v1", "output": good})
        fake = _make_fake(capture, canned, self.real_bashy)
        old = dict(os.environ)
        try:
            os.environ["BASHY_BIN"] = fake
            os.environ["TUTOR_AGENT"] = "env-agent"
            # do NOT pass --agent, should use $TUTOR_AGENT
            proc = self.tutor("nudge", str(self.sprint), "--person", "alice", "--json", check=False)
        finally:
            os.environ.clear()
            os.environ.update(old)
        os.unlink(fake)
        prompt = Path(capture).read_text(encoding="utf-8") if Path(capture).exists() else ""
        if Path(capture).exists():
            Path(capture).unlink()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        data = json.loads(proc.stdout)
        self.assertEqual(data.get("hint").strip(), good)
        self.assertIn(title, prompt)


if __name__ == "__main__":
    unittest.main()
