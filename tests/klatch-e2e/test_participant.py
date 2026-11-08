"""Offline checks of the participant journey helper run.py imports."""
import importlib.util
import os
import stat
import tempfile
import types
import unittest
from pathlib import Path
from unittest import mock

SPEC = importlib.util.spec_from_file_location("klatch_participant", Path(__file__).with_name("participant.py"))
p = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(p)


class FakeJourney:
    def __init__(self, args, url):
        self.args, self.url = args, url
        self.calls = 3
        self.skipped = []
        self.closed = False
        FakeJourney.last = self

    def close(self):
        self.closed = True
        return {"status": "done", "detail": "removed journey tree"}


def make_args(tmp, **kw):
    helper = Path(tmp) / "main.js"
    helper.write_text("// helper")
    base = dict(helper=str(helper), node="node", node_path="/usr/bin/node",
                wrapper=str(Path(tmp) / "klatch.bsh"), keep_tree=False)
    base.update(kw)
    return types.SimpleNamespace(**base)


URL = "http://127.0.0.1:7100/"


def passing(name):
    return lambda sb, ctx: ({}, [f"{name}_ok"])


def all_passing(**over):
    funcs = {name: passing(name) for name in p.STEPS}
    funcs.update(over)
    return funcs


def boom(sb, ctx):
    raise p.Check("broken")


def blocked(sb, ctx):
    raise p.Blocked("not here")


class Decisions(unittest.TestCase):
    def test_all_steps_pass(self):
        with tempfile.TemporaryDirectory() as d:
            out = p.run_journey(make_args(d), URL, all_passing(), FakeJourney)
        self.assertEqual(p.summarize(out["steps"]), "PASS")
        self.assertEqual(out["checks_passed"], len(p.STEPS))
        self.assertEqual(out["cli_calls"], 3)
        self.assertEqual(out["journey_teardown"]["status"], "done")
        self.assertTrue(FakeJourney.last.closed)
        self.assertEqual(len(out["accounts"]), 2)

    def test_journey_uses_the_given_url_and_never_a_server(self):
        with tempfile.TemporaryDirectory() as d:
            p.run_journey(make_args(d), URL, all_passing(), FakeJourney)
        self.assertEqual(FakeJourney.last.url, URL)
        self.assertFalse(hasattr(p, "Sandbox"))
        self.assertFalse(hasattr(p, "main"))

    def test_preflight_blocks_missing_helper_and_node(self):
        with tempfile.TemporaryDirectory() as d:
            self.assertIsNone(p.preflight(make_args(d)))
            self.assertIn("participant helper not built",
                          p.preflight(make_args(d, helper=str(Path(d) / "absent.js"))))
            self.assertIn("node not found", p.preflight(make_args(d, node_path=None)))

    def test_failure_blocks_dependents_but_logout_still_cleans_up(self):
        with tempfile.TemporaryDirectory() as d:
            out = p.run_journey(make_args(d), URL, all_passing(messages=boom), FakeJourney)
        s = out["steps"]
        self.assertEqual(p.summarize(s), "FAIL")
        self.assertEqual(s["messages"]["status"], "fail")
        self.assertEqual(s["attachments"]["status"], "blocked")
        self.assertEqual(s["wrapper"]["reason"], "skipped: messages failed")
        self.assertEqual(s["logout"]["status"], "pass")

    def test_provision_failure_skips_logout_and_closes(self):
        with tempfile.TemporaryDirectory() as d:
            out = p.run_journey(make_args(d), URL, all_passing(provision=boom), FakeJourney)
        self.assertEqual(out["steps"]["logout"]["status"], "blocked")
        self.assertEqual(out["journey_teardown"]["status"], "done")

    def test_blocked_wrapper_is_never_a_pass_and_does_not_stop_logout(self):
        with tempfile.TemporaryDirectory() as d:
            out = p.run_journey(make_args(d), URL, all_passing(wrapper=blocked), FakeJourney)
        self.assertEqual(p.summarize(out["steps"]), "BLOCKED")
        self.assertEqual(out["steps"]["logout"]["status"], "pass")

    def test_watch_failure_fails_run_but_wrapper_still_runs(self):
        with tempfile.TemporaryDirectory() as d:
            out = p.run_journey(make_args(d), URL, all_passing(watch=boom), FakeJourney)
        self.assertEqual(p.summarize(out["steps"]), "FAIL")
        self.assertEqual(out["steps"]["wrapper"]["status"], "pass")

    def test_skipped_posix_checks_are_not_counted(self):
        def step(sb, ctx):
            sb.skipped.append("session_0600")
            return {}, ["a", None]
        with tempfile.TemporaryDirectory() as d:
            out = p.run_journey(make_args(d), URL, all_passing(login=step), FakeJourney)
        self.assertEqual(out["steps"]["login"]["checks"], ["a"])
        self.assertEqual(out["skipped_checks"], ["session_0600"])
        self.assertEqual(out["checks_passed"], len(p.STEPS))

    def test_journey_setup_failure_is_fail(self):
        def factory(args, url):
            raise OSError("no tmp")
        with tempfile.TemporaryDirectory() as d:
            out = p.run_journey(make_args(d), URL, all_passing(), factory)
        self.assertEqual(p.summarize(out["steps"]), "FAIL")


class JourneyTree(unittest.TestCase):
    def test_private_tree_removed_and_url_normalized(self):
        with tempfile.TemporaryDirectory() as d:
            jr = p.Journey(make_args(d), URL)
            self.assertEqual(jr.url, "http://127.0.0.1:7100")
            root = jr.root
            self.assertTrue(root.is_dir())
            self.assertEqual(jr.close()["status"], "done")
            self.assertFalse(root.exists())

    def test_claimed_store_is_blocked_before_provisioning(self):
        with tempfile.TemporaryDirectory() as d:
            jr = p.Journey(make_args(d), URL)
            try:
                with mock.patch.object(jr, "admin_status", return_value=False), \
                        mock.patch.object(p, "load_provision") as lp:
                    with self.assertRaises(p.Blocked) as cm:
                        jr.provision(["x", "y"])
                lp.assert_not_called()
                self.assertIn("already claimed", str(cm.exception))
            finally:
                jr.close()

    def test_posix_check(self):
        with tempfile.TemporaryDirectory() as d:
            jr = p.Journey(make_args(d), URL)
            try:
                with mock.patch.object(p, "POSIX", True):
                    self.assertEqual(jr.posix_check("m", lambda: True, "bad"), "m")
                    with self.assertRaises(p.Check):
                        jr.posix_check("m", lambda: False, "bad")
                with mock.patch.object(p, "POSIX", False):
                    self.assertIsNone(jr.posix_check("m", lambda: False, "bad"))
                    jr.posix_check("m", lambda: False, "bad")
                self.assertEqual(jr.skipped, ["m"])
            finally:
                jr.close()


class Helpers(unittest.TestCase):
    def test_leaked_reports_names_never_values(self):
        names = p.leaked("x hunter2 y", {"alice_password": "hunter2", "seal": "other", "empty": ""})
        self.assertEqual(names, ["alice_password"])

    @unittest.skipUnless(os.name != "nt", "POSIX file modes")
    def test_private_write_is_0600_and_refuses_existing(self):
        with tempfile.TemporaryDirectory() as d:
            f = os.path.join(d, "in.json")
            p.private_write(f, "{}")
            self.assertEqual(stat.S_IMODE(os.stat(f).st_mode), 0o600)
            with self.assertRaises(FileExistsError):
                p.private_write(f, "{}")

    def test_helper_dist_digest_sees_sibling_modules(self):
        with tempfile.TemporaryDirectory() as d:
            (Path(d) / "main.js").write_text("import './commands.js'\n")
            (Path(d) / "commands.js").write_text("old\n")
            before = p.sha256_tree(d)
            (Path(d) / "commands.js").write_text("fixed\n")
            self.assertNotEqual(before, p.sha256_tree(d))
            info = p.candidate(types.SimpleNamespace(helper=str(Path(d) / "main.js"), wrapper=None,
                                                     node="/nonexistent-node", helper_rev="r"))
            self.assertEqual(info["helper_dist_sha256"], p.sha256_tree(d))
            self.assertEqual(info["wrapper_sha256"], "unknown")

    def test_wrapper_detection(self):
        with tempfile.TemporaryDirectory() as d:
            w = Path(d) / "klatch.bsh"
            w.write_text("setup|provision|status only\n")
            self.assertFalse(p.wrapper_has_participant_verbs(w))
            w.write_text("KLATCH_HELPER messages-send post-file\n")
            self.assertTrue(p.wrapper_has_participant_verbs(w))
            self.assertFalse(p.wrapper_has_participant_verbs(Path(d) / "absent"))

    def test_step_wrapper_blocks_on_setup_only_wrapper(self):
        with tempfile.TemporaryDirectory() as d:
            w = Path(d) / "klatch.bsh"
            w.write_text("setup|provision|status only\n")
            sb = types.SimpleNamespace(args=types.SimpleNamespace(wrapper=str(w)))
            with self.assertRaises(p.Blocked) as cm:
                p.step_wrapper(sb, {})
            self.assertIn("296fa5066f54", str(cm.exception))

    def test_result_shapes(self):
        cards = [{"id": "c1", "data": {"cardProfile": {"guid": "g1"}, "cardDetail": {"status": "connected"}}}]
        self.assertEqual(p.card_for(cards, "g1")["id"], "c1")
        self.assertIsNone(p.card_for(cards, "g2"))
        self.assertEqual(p.card_status(cards[0]), "connected")
        topics = [{"id": "t1", "data": {"topicDetail": {"guid": "g1", "data": {"text": "[Assistant: A] hi"}}}}]
        t = p.find_topic(topics, "t1")
        self.assertEqual(p.topic_text(t), "[Assistant: A] hi")
        self.assertEqual(p.topic_guid(t), "g1")
        self.assertIsNone(p.topic_text(p.find_topic(topics, "t2")))

    def test_revocation_answer(self):
        import urllib.error
        before = {"seal": {"publicKey": "K"}}

        def refuse(code):
            def f():
                raise urllib.error.HTTPError("u", code, "x", {}, None)
            return f
        self.assertEqual(p.revocation_answer(before, refuse(401)), "refused")
        self.assertEqual(p.revocation_answer(before, refuse(404)), "refused")
        with self.assertRaises(p.Check):
            p.revocation_answer(before, refuse(500))
        self.assertEqual(p.revocation_answer(before, lambda: {"seal": {}}), "empty")
        with self.assertRaises(p.Check):
            p.revocation_answer(before, lambda: {"seal": {"publicKey": "K"}})
        with self.assertRaises(p.Check):
            p.revocation_answer({}, lambda: {"seal": {}})

    def test_summarize_pending_is_not_a_pass(self):
        steps = {name: {"status": "pass"} for name in p.STEPS}
        steps["watch"] = {"status": "pending"}
        self.assertEqual(p.summarize(steps), "BLOCKED")


if __name__ == "__main__":
    unittest.main()
