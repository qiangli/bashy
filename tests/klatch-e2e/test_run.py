"""Offline checks of the klatch-e2e runner's pass/fail/BLOCKED decisions."""
import importlib.util
import json
import os
import subprocess
import tempfile
from datetime import datetime
from pathlib import Path
import types
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("klatch_e2e_run", Path(__file__).with_name("run.py"))
runner = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runner)


def make_args(topology="none", url=None, image=None, topic="General",
              expect_topics=("Team",), evidence=None):
    return types.SimpleNamespace(topology=topology, url=url, image=image, topic=topic,
                                 expect_topics=list(expect_topics), evidence=evidence)


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


def pass_hooks_for(fake):
    """PASS_HOOKS with an agent post under the agent's own identity."""
    def agent_post(url, agent, agent_tag):
        msg = f"klatch-e2e agent {agent_tag}"
        fake.posted.append(msg)
        agent_id = agent.get("agent_id") if isinstance(agent, dict) else agent_tag
        return {"sender": agent_id, "message": msg}

    return {
        "outpost_reachable": lambda: True,
        "start_server": lambda topology, url, image: {"container": "test-container", "image": image or "klatch:test"},
        "stop_server": lambda server: {"status": "done", "detail": f"removed container {server['container']}"},
        "register_agent": lambda url, tag: {"agent_id": tag, "account": "agent"},
        "agent_post": agent_post,
        "ensure_meet": lambda url: {"meet_id": "meet-1"},
        "candidate_info": lambda: {"bashy_version": "v-test", "bashy_sha": "sha-test"},
        "teardown": lambda topology, ctx: {"status": "done", "detail": "test teardown"},
    }


def run_with(fake, args=None, hooks=None):
    merged = pass_hooks_for(fake)
    merged.update(hooks or {})
    with mock.patch.object(runner, "browser", fake), mock.patch.object(runner.time, "sleep", lambda s: None), \
            mock.patch.object(runner, "wait_for", lambda fn, what, timeout=20: fn() or (_ for _ in ()).throw(AssertionError(f"timed out waiting for {what}"))):
        return runner.run(args or make_args(), hooks=merged)


class RunnerDecisions(unittest.TestCase):
    def test_pass_posts_and_reads_back(self):
        fake = FakeBrowser()
        rec = run_with(fake)
        self.assertEqual(rec["result"], "PASS")
        self.assertEqual(rec["steps"]["signup_login"]["status"], "pass")
        posted = rec["steps"]["topic_exchange"]["detail"]["posted"]
        self.assertIn("klatch-e2e human", posted["human"])
        self.assertIn("klatch-e2e agent", posted["agent"])
        self.assertNotEqual(posted["human"], posted["agent"])
        self.assertEqual(rec["steps"]["topic_exchange"]["detail"]["agent_sender"],
                         rec["steps"]["signup_login"]["detail"]["agent"]["agent_id"])

    def test_blocked_without_hub(self):
        rec = run_with(FakeBrowser(hub=False))
        self.assertEqual(rec["result"], "BLOCKED")
        self.assertEqual(rec["steps"]["signup_login"]["status"], "blocked")

    def test_blocked_on_signin_gate(self):
        rec = run_with(FakeBrowser(title="Sign in to continue"))
        self.assertEqual(rec["result"], "BLOCKED")

    def test_fail_when_klatch_login_form_shown(self):
        rec = run_with(FakeBrowser(login_form=True))
        self.assertEqual(rec["result"], "FAIL")
        self.assertIn("login form", rec["steps"]["signup_login"]["reason"])

    def test_fail_when_team_topic_missing(self):
        rec = run_with(FakeBrowser(topics=("General",)))
        self.assertEqual(rec["result"], "FAIL")
        self.assertIn("topics not listed", rec["steps"]["topic_exchange"]["reason"])

    def test_fail_when_message_never_appears(self):
        rec = run_with(FakeBrowser(send_works=False))
        self.assertEqual(rec["result"], "FAIL")
        self.assertIn("message", rec["steps"]["topic_exchange"]["reason"])

    def test_fail_when_agent_sender_mismatched(self):
        def wrong_sender(url, agent, agent_tag):
            return {"sender": "someone-else", "message": f"klatch-e2e agent {agent_tag}"}

        rec = run_with(FakeBrowser(), hooks={"agent_post": wrong_sender})
        self.assertEqual(rec["result"], "FAIL")
        self.assertIn("attribution", rec["steps"]["topic_exchange"]["reason"])

    def test_default_agent_post_is_blocked_with_sprint(self):
        rec = run_with(FakeBrowser(), hooks={"agent_post": runner.default_agent_post})
        self.assertEqual(rec["result"], "BLOCKED")
        self.assertEqual(rec["steps"]["signup_login"]["status"], "pass")
        self.assertEqual(rec["steps"]["topic_exchange"]["status"], "blocked")
        self.assertIn("S568", rec["steps"]["topic_exchange"]["reason"])


class TopologySelection(unittest.TestCase):
    def test_parser_accepts_all_topologies(self):
        for topology in ("none", "outpost", "satellite", "both"):
            args = runner.build_parser().parse_args(["--topology", topology])
            self.assertEqual(args.topology, topology)

    def test_parser_rejects_unknown_topology(self):
        with self.assertRaises(SystemExit):
            runner.build_parser().parse_args(["--topology", "mars"])

    def test_parser_defaults_to_none_without_hardcoded_url(self):
        args = runner.build_parser().parse_args([])
        self.assertEqual(args.topology, "none")
        self.assertIsNone(args.url)
        self.assertFalse(hasattr(runner, "DEFAULT_URL"))

    def test_none_uses_local_url(self):
        rec = run_with(FakeBrowser(), args=make_args(topology="none"))
        self.assertEqual(rec["url"], runner.LOCAL_URL)
        self.assertEqual(rec["url_source"], "local-default")

    def test_none_starts_and_stops_container(self):
        seen = {}

        def start(topology, url, image):
            seen["start"] = (topology, url, image)
            return {"container": "c1", "image": "klatch:test"}

        def stop(server):
            seen["stop"] = server
            return {"status": "done", "detail": "removed container c1"}

        rec = run_with(FakeBrowser(), hooks={"start_server": start, "stop_server": stop})
        self.assertEqual(rec["result"], "PASS")
        self.assertEqual(seen["start"][0], "none")
        self.assertEqual(seen["stop"], {"container": "c1", "image": "klatch:test"})
        self.assertEqual(rec["server"], {"container": "c1", "image": "klatch:test"})
        self.assertEqual(rec["server_stop"]["status"], "done")
        self.assertIn("removed container c1", rec["teardown"]["detail"])

    def test_none_container_stops_even_when_blocked(self):
        stopped = []

        rec = run_with(FakeBrowser(hub=False),
                       hooks={"stop_server": lambda server: stopped.append(server) or {"status": "done", "detail": "x"}})
        self.assertEqual(rec["result"], "BLOCKED")
        self.assertEqual(len(stopped), 1)

    def test_none_oci_unavailable_is_blocked(self):
        def boom(topology, url, image):
            raise runner.Blocked("bashy oci unavailable: krunkit missing")

        rec = run_with(FakeBrowser(), hooks={"start_server": boom})
        self.assertEqual(rec["result"], "BLOCKED")
        for name in runner.STEPS:
            self.assertEqual(rec["steps"][name]["status"], "blocked")
        self.assertIn("oci", rec["steps"]["signup_login"]["reason"])

    def test_satellite_missing_url_is_blocked(self):
        with mock.patch.dict(os.environ, {}, clear=False):
            os.environ.pop("KLATCH_E2E_URL", None)
            rec = run_with(FakeBrowser(), args=make_args(topology="satellite"))
        self.assertEqual(rec["result"], "BLOCKED")
        for name in runner.STEPS:
            self.assertEqual(rec["steps"][name]["status"], "blocked")
        self.assertIn("KLATCH_E2E_URL", rec["steps"]["signup_login"]["reason"])
        self.assertEqual(rec["url_source"], "missing")
        self.assertIsNone(rec["url"])

    def test_both_missing_url_is_blocked(self):
        with mock.patch.dict(os.environ, {}, clear=False):
            os.environ.pop("KLATCH_E2E_URL", None)
            rec = run_with(FakeBrowser(), args=make_args(topology="both"))
        self.assertEqual(rec["result"], "BLOCKED")
        self.assertEqual(rec["url_source"], "missing")

    def test_outpost_missing_url_is_blocked_not_crash(self):
        with mock.patch.dict(os.environ, {}, clear=False):
            os.environ.pop("KLATCH_E2E_URL", None)
            rec = run_with(FakeBrowser(), args=make_args(topology="outpost"))
        self.assertEqual(rec["result"], "BLOCKED")
        for name in runner.STEPS:
            self.assertEqual(rec["steps"][name]["status"], "blocked")
        self.assertIn("KLATCH_E2E_URL", rec["steps"]["signup_login"]["reason"])

    def test_satellite_accepts_env_url(self):
        with mock.patch.dict(os.environ, {"KLATCH_E2E_URL": "https://klatch.example.test/"}):
            rec = run_with(FakeBrowser(), args=make_args(topology="satellite"))
        self.assertEqual(rec["result"], "PASS")
        self.assertEqual(rec["url"], "https://klatch.example.test/")
        self.assertEqual(rec["url_source"], "env")

    def test_satellite_accepts_cli_url(self):
        rec = run_with(FakeBrowser(), args=make_args(topology="satellite", url="https://klatch.example.test/"))
        self.assertEqual(rec["result"], "PASS")
        self.assertEqual(rec["url_source"], "cli")

    def test_outpost_unreachable_is_blocked_with_sprint(self):
        rec = run_with(FakeBrowser(), args=make_args(topology="outpost", url="https://klatch.example.test/"),
                       hooks={"outpost_reachable": lambda: False})
        self.assertEqual(rec["result"], "BLOCKED")
        self.assertIn("S565", rec["steps"]["signup_login"]["reason"])

    def test_outpost_reachable_needs_klatch_app(self):
        apps = ("NAME SCHEME ENABLED TARGET EXPOSE FLAGS\n"
                "llm http true 127.0.0.1:24556 - \n")

        def fake_run(argv, **kwargs):
            cmd = argv[-2:]
            if cmd == ["outpost", "status"]:
                return types.SimpleNamespace(returncode=0, stdout="", stderr="")
            return types.SimpleNamespace(returncode=0, stdout=apps, stderr="")

        with mock.patch.object(runner.subprocess, "run", side_effect=fake_run):
            self.assertFalse(runner.default_outpost_reachable())

        apps_on = apps + "klatch http true 127.0.0.1:7100 - \n"

        def fake_run_on(argv, **kwargs):
            cmd = argv[-2:]
            if cmd == ["outpost", "status"]:
                return types.SimpleNamespace(returncode=0, stdout="", stderr="")
            return types.SimpleNamespace(returncode=0, stdout=apps_on, stderr="")

        with mock.patch.object(runner.subprocess, "run", side_effect=fake_run_on):
            self.assertTrue(runner.default_outpost_reachable())

    def test_agent_registration_missing_is_blocked_with_sprint(self):
        rec = run_with(FakeBrowser(), hooks={"register_agent": runner.default_register_agent})
        self.assertEqual(rec["result"], "BLOCKED")
        self.assertIn("S568", rec["steps"]["signup_login"]["reason"])

    def test_meet_missing_is_blocked_with_sprint(self):
        rec = run_with(FakeBrowser(), hooks={"ensure_meet": runner.default_ensure_meet})
        self.assertEqual(rec["result"], "BLOCKED")
        self.assertEqual(rec["steps"]["signup_login"]["status"], "pass")
        self.assertEqual(rec["steps"]["topic_exchange"]["status"], "pass")
        self.assertIn("S568", rec["steps"]["meet_session"]["reason"])


class LocalServerDefaults(unittest.TestCase):
    def test_start_skips_non_none(self):
        self.assertIsNone(runner.default_start_server("satellite", "https://x/", None))

    def test_oci_engine_failure_is_blocked(self):
        bad = types.SimpleNamespace(returncode=1, stdout="", stderr="boom")

        with mock.patch.object(runner.subprocess, "run", return_value=bad):
            with self.assertRaises(runner.Blocked) as ctx:
                runner.default_start_server("none", runner.LOCAL_URL, "klatch:test")
        self.assertIn("oci", str(ctx.exception))

    def test_oci_not_running_is_blocked(self):
        bad = types.SimpleNamespace(returncode=0, stdout="[]",
                                    stderr="bashy podman: bashy's podman is not running (...)")

        with mock.patch.object(runner.subprocess, "run", return_value=bad):
            with self.assertRaises(runner.Blocked) as ctx:
                runner.default_start_server("none", runner.LOCAL_URL, "klatch:test")
        self.assertIn("oci", str(ctx.exception))

    def test_missing_image_is_blocked(self):
        empty = types.SimpleNamespace(returncode=0, stdout="[]", stderr="")

        with mock.patch.dict(os.environ, {}, clear=False):
            os.environ.pop("KLATCH_E2E_IMAGE", None)
            with mock.patch.object(runner.subprocess, "run", return_value=empty):
                with self.assertRaises(runner.Blocked) as ctx:
                    runner.default_start_server("none", runner.LOCAL_URL, None)
        self.assertIn("--image", str(ctx.exception))

    def test_find_klatch_image(self):
        self.assertIsNone(runner.find_klatch_image("[]"))
        self.assertIsNone(runner.find_klatch_image("not json"))
        dockery = json.dumps([{"Repository": "klatch", "Tag": "1.2.6"}])
        self.assertEqual(runner.find_klatch_image(dockery), "klatch:1.2.6")
        podmany = json.dumps([{"Names": ["localhost/klatch:latest", "localhost/other:1"]}])
        self.assertEqual(runner.find_klatch_image(podmany), "localhost/klatch:latest")

    def test_stop_server_never_raises(self):
        with mock.patch.object(runner.subprocess, "run", side_effect=OSError("nope")):
            info = runner.default_stop_server({"container": "c1", "image": "i"})
        self.assertEqual(info["status"], "failed")
        self.assertEqual(runner.default_stop_server(None)["status"], "skipped")


class CandidateIdentity(unittest.TestCase):
    def test_parse_bashy_version(self):
        version, sha = runner.parse_bashy_version(
            "bashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy-dev (f1bcd71)\n")
        self.assertIn("bashy", version)
        self.assertEqual(sha, "f1bcd71")

    def test_parse_bashy_version_unknown(self):
        self.assertEqual(runner.parse_bashy_version(""), ("unknown", "unknown"))

    def test_candidate_uses_bashy_binary(self):
        out = types.SimpleNamespace(returncode=0, stdout="bashy test (abc1234)\n", stderr="")

        with mock.patch.dict(os.environ, {"BASHY_BIN": "/tmp/fake-bashy"}):
            with mock.patch.object(runner.subprocess, "run", return_value=out) as m:
                info = runner.candidate_info()
        self.assertEqual(m.call_args[0][0][:2], ["/tmp/fake-bashy", "--version"])
        self.assertEqual(info["bashy_version"], "bashy test (abc1234)")
        self.assertEqual(info["bashy_sha"], "abc1234")

    def test_candidate_unknown_when_binary_missing(self):
        with mock.patch.object(runner.subprocess, "run", side_effect=OSError("nope")):
            info = runner.candidate_info()
        self.assertEqual(info, {"bashy_version": "unknown", "bashy_sha": "unknown"})


class ExitContract(unittest.TestCase):
    def test_blocked_is_never_counted_as_pass(self):
        for name in runner.STEPS:
            steps = {n: {"status": "pass"} for n in runner.STEPS}
            steps[name] = {"status": "blocked", "reason": "missing support"}
            self.assertEqual(runner.summarize(steps), "BLOCKED")

    def test_fail_beats_blocked(self):
        steps = {"signup_login": {"status": "fail", "reason": "x"},
                 "topic_exchange": {"status": "blocked", "reason": "y"},
                 "meet_session": {"status": "blocked", "reason": "z"}}
        self.assertEqual(runner.summarize(steps), "FAIL")

    def test_exit_codes(self):
        self.assertEqual(runner.exit_code_for("PASS"), 0)
        self.assertEqual(runner.exit_code_for("FAIL"), 1)
        self.assertEqual(runner.exit_code_for("BLOCKED"), 2)

    def test_main_returns_blocked_without_url(self):
        with mock.patch.dict(os.environ, {}, clear=False):
            os.environ.pop("KLATCH_E2E_URL", None)
            with mock.patch("builtins.print"):
                code = runner.main(["--topology", "satellite"])
        self.assertEqual(code, 2)

    def test_main_outpost_without_url_is_blocked_not_crash(self):
        with mock.patch.dict(os.environ, {}, clear=False):
            os.environ.pop("KLATCH_E2E_URL", None)
            with mock.patch("builtins.print"):
                code = runner.main(["--topology", "outpost"])
        self.assertEqual(code, 2)


class EvidenceSchema(unittest.TestCase):
    def full_pass_record(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = str(Path(tmp) / "evidence.json")
            rec = run_with(FakeBrowser(), args=make_args(topology="none", evidence=path))
            with open(path, encoding="utf-8") as f:
                on_disk = json.load(f)
        return rec, on_disk

    def test_evidence_record_schema(self):
        rec, _ = self.full_pass_record()
        for key in ("topology", "url", "url_source", "steps", "result", "candidate",
                    "started_at", "finished_at", "teardown", "accounts", "server", "server_stop"):
            self.assertIn(key, rec)
        self.assertEqual(rec["topology"], "none")
        self.assertEqual(set(rec["steps"]), set(runner.STEPS))
        for name in runner.STEPS:
            self.assertIn(rec["steps"][name]["status"], ("pass", "fail", "blocked"))
        self.assertIn(rec["result"], ("PASS", "FAIL", "BLOCKED"))
        for key in ("bashy_version", "bashy_sha"):
            self.assertIn(key, rec["candidate"])
        datetime.fromisoformat(rec["started_at"])
        datetime.fromisoformat(rec["finished_at"])
        self.assertNotEqual(rec["accounts"]["human_tag"], rec["accounts"]["agent_tag"])

    def test_evidence_file_matches_record(self):
        rec, on_disk = self.full_pass_record()
        for key in ("topology", "result", "steps", "candidate", "server"):
            self.assertEqual(on_disk[key], rec[key])

    def test_teardown_runs_and_is_recorded(self):
        seen = []

        def teardown(topology, ctx):
            seen.append((topology, ctx))
            return {"status": "done", "detail": "removed sandbox and test accounts"}

        rec = run_with(FakeBrowser(), hooks={"teardown": teardown})
        self.assertEqual(len(seen), 1)
        self.assertEqual(seen[0][0], "none")
        self.assertIn("agent_tag", seen[0][1])
        self.assertEqual(rec["teardown"]["status"], "done")


if __name__ == "__main__":
    unittest.main()
