"""Offline checks of the klatch-e2e runner's pass/fail/BLOCKED decisions."""
import importlib.util
import json
from pathlib import Path
import types
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("klatch_e2e_run", Path(__file__).with_name("run.py"))
runner = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runner)

ARGS = types.SimpleNamespace(url="https://sat/klatch/", topic="General", expect_topics=["Team"])


class FakeBrowser:
    """Stands in for `bashy browser --mode live` with a scripted page."""

    def __init__(self, hub=True, title="Klatch", session=True, login_form=False, topics=("General", "Team"), send_works=True):
        self.hub, self.title, self.session = hub, title, session
        self.login_form, self.topics, self.send_works = login_form, topics, send_works
        self.box, self.posted = "", []

    def body(self):
        return "\n".join(list(self.topics) + self.posted)

    def __call__(self, *args, tab=None, json_out=False, timeout=90):
        verb = args[0]
        if verb == "status":
            return {"hub_up": self.hub, "extension_connected": self.hub, "extension_version": "0.10.1"}
        if verb == "navigate":
            return {"tab_id": 7, "title": self.title}
        if verb == "type":
            self.box = args[2]
            return "typed"
        if verb == "click":
            if args[1] == runner.SEND_ICON and self.send_works:
                self.posted.append(self.box)
                self.box = ""
            return "clicked"
        if verb == "eval":
            expr = args[1]
            if "location.hash" in expr:
                return json.dumps(self.session)
            if "type=password" in expr:
                return json.dumps(self.login_form)
            if "includes(" in expr:
                return json.dumps(any(m and m in expr for m in self.posted))
            if "querySelector('textarea')||{}" in expr:
                return json.dumps(self.box)
            if "querySelector('textarea')" in expr:
                return json.dumps(True)
            if "innerText" in expr:
                return json.dumps(self.body())
        raise AssertionError(f"unexpected browser call {args}")


def run_with(fake):
    with mock.patch.object(runner, "browser", fake), mock.patch.object(runner.time, "sleep", lambda s: None), \
            mock.patch.object(runner, "wait_for", lambda fn, what, timeout=20: fn() or (_ for _ in ()).throw(AssertionError(f"timed out waiting for {what}"))):
        return runner.run(ARGS)


class RunnerDecisions(unittest.TestCase):
    def test_pass_posts_and_reads_back(self):
        ev = run_with(FakeBrowser())
        self.assertTrue(ev["posted"].startswith("klatch-e2e "))
        self.assertEqual(ev["tab_id"], 7)

    def test_blocked_without_hub(self):
        with self.assertRaises(runner.Blocked):
            run_with(FakeBrowser(hub=False))

    def test_blocked_on_signin_gate(self):
        with self.assertRaises(runner.Blocked):
            run_with(FakeBrowser(title="Sign in to continue"))

    def test_fail_when_klatch_login_form_shown(self):
        with self.assertRaisesRegex(AssertionError, "login form"):
            run_with(FakeBrowser(login_form=True))

    def test_fail_when_team_topic_missing(self):
        with self.assertRaisesRegex(AssertionError, "topics not listed"):
            run_with(FakeBrowser(topics=("General",)))

    def test_fail_when_message_never_appears(self):
        with self.assertRaisesRegex(AssertionError, "posted message"):
            run_with(FakeBrowser(send_works=False))


if __name__ == "__main__":
    unittest.main()
