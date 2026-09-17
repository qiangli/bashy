"""Deterministic, offline unit tests for the bounded mini-swe-agent harness.

No paid/network model call: the ReplayModel scripts steps and the live transport
is covered separately against a loopback fake provider (fixtures/live_check.py).
Run from the harness/ directory:

    python3 -m unittest discover -s tests
"""

from __future__ import annotations

import json
import os
import unittest
from pathlib import Path

from minisweagent_bounded import TRAJECTORY_FORMAT, UPSTREAM_VERSION
from minisweagent_bounded.agent import InteractiveAgent, InteractiveAgentConfig
from minisweagent_bounded.cli import UsageError, build_spec, _build_parser
from minisweagent_bounded.config import (
    ConfigError,
    load_config,
    validate_budget,
    validate_cost_per_call,
    validate_replay_steps,
)
from minisweagent_bounded.environment import BashyNotFound, LocalEnvironment, resolve_bashy
from minisweagent_bounded.exceptions import NonInteractiveApproval, Submitted, UserInterruption
from minisweagent_bounded.model import FormatError, ReplayModel, extract_action
from minisweagent_bounded.prompter import ScriptedPrompter
from minisweagent_bounded.runner import normalized_envelope, run_scenario

FIXTURES = Path(__file__).resolve().parents[2] / "fixtures"
SCENARIOS = FIXTURES / "scenarios"
GOLDENS = FIXTURES / "goldens"

SCENARIO_NAMES = ["submit-success", "cost-limit", "step-limit", "repeated-format-error", "format-error-recovers"]


def _has_path_bashy() -> bool:
    import shutil
    return shutil.which("bashy") is not None


HAVE_BASHY = bool(os.environ.get("BASHY_BIN")) or _has_path_bashy()


# ---------------------------------------------------------------- scenarios ---
class ScenarioGoldenTest(unittest.TestCase):
    @unittest.skipUnless(HAVE_BASHY, "requires a Bashy executor")
    def test_scenarios_match_goldens(self):
        for name in SCENARIO_NAMES:
            with self.subTest(scenario=name):
                spec = json.loads((SCENARIOS / name).with_suffix(".json").read_text())
                got = normalized_envelope(spec, run_scenario(spec))
                want = json.loads((GOLDENS / name).with_suffix(".json").read_text())
                self.assertEqual(got, want)

    @unittest.skipUnless(HAVE_BASHY, "requires a Bashy executor")
    def test_deterministic(self):
        spec = json.loads((SCENARIOS / "submit-success.json").read_text())
        self.assertEqual(normalized_envelope(spec, run_scenario(spec)),
                         normalized_envelope(spec, run_scenario(spec)))


# -------------------------------------------------------------- environment ---
class EnvironmentTest(unittest.TestCase):
    @unittest.skipUnless(HAVE_BASHY, "requires a Bashy executor")
    def test_runs_through_bashy_argv(self):
        env = LocalEnvironment()
        out = env.execute({"command": "echo hi"})
        self.assertEqual(out["returncode"], 0)
        self.assertIn("hi", out["output"])

    @unittest.skipUnless(HAVE_BASHY, "requires a Bashy executor")
    def test_actions_are_stateless(self):
        env = LocalEnvironment()
        env.execute({"command": "cd /"})
        out = env.execute({"command": "pwd"})
        self.assertNotEqual(out["output"].strip(), "/")

    @unittest.skipUnless(HAVE_BASHY, "requires a Bashy executor")
    def test_submit_protocol(self):
        env = LocalEnvironment()
        with self.assertRaises(Submitted) as ctx:
            env.execute({"command": "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\necho payload"})
        self.assertEqual(ctx.exception.messages[0]["extra"]["submission"], "payload\n")

    @unittest.skipUnless(HAVE_BASHY, "requires a Bashy executor")
    def test_marker_not_first_line_does_not_submit(self):
        env = LocalEnvironment()
        out = env.execute({"command": "echo before\necho COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"})
        self.assertEqual(out["returncode"], 0)

    @unittest.skipUnless(HAVE_BASHY, "requires a Bashy executor")
    def test_timeout_is_reported(self):
        env = LocalEnvironment(timeout=1)
        out = env.execute({"command": "sleep 30"})
        self.assertEqual(out["returncode"], -1)
        self.assertIn("error", out["exception_info"].lower())

    def test_no_host_shell_fallback(self):
        with self.assertRaises(BashyNotFound):
            resolve_bashy_with_bogus()


def resolve_bashy_with_bogus():
    old = os.environ.get("BASHY_BIN")
    os.environ["BASHY_BIN"] = "/nonexistent/bashy-xyz"
    try:
        return resolve_bashy()
    finally:
        if old is None:
            del os.environ["BASHY_BIN"]
        else:
            os.environ["BASHY_BIN"] = old


# ------------------------------------------------------------------- config ---
class ConfigTest(unittest.TestCase):
    def test_key_value_merge(self):
        cfg = load_config(["config.cost_limit=1.5", "config.step_limit=4"])
        self.assertEqual(cfg, {"config": {"cost_limit": 1.5, "step_limit": 4}})

    def test_json_file_and_override(self, ):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp) / "c.json"
            p.write_text(json.dumps({"config": {"cost_limit": 2.0, "step_limit": 1}}))
            cfg = load_config([str(p), "config.step_limit=9"])
            self.assertEqual(cfg["config"], {"cost_limit": 2.0, "step_limit": 9})

    def test_malformed_json_rejected(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp) / "bad.json"
            p.write_text("{not json")
            with self.assertRaises(ConfigError):
                load_config([str(p)])

    def test_budget_validation(self):
        self.assertEqual(validate_budget(cost_limit=0, step_limit=0), (0.0, 0))
        for bad in ("nan", "inf", "-1"):
            with self.assertRaises(ConfigError):
                validate_budget(cost_limit=bad, step_limit=0)
        with self.assertRaises(ConfigError):
            validate_budget(cost_limit=1.0, step_limit=-2)

    def test_cost_per_call_validation(self):
        self.assertEqual(validate_cost_per_call(0.01), 0.01)
        for bad in (float("nan"), float("inf"), -1):
            with self.assertRaises(ConfigError):
                validate_cost_per_call(bad)

    def test_replay_step_validation(self):
        validate_replay_steps([{"command": "echo hi"}, {"format_error": "x"}])
        for bad in (["not a dict"], [{"reasoning": "no action"}], [{"command": 5}]):
            with self.assertRaises(ConfigError):
                validate_replay_steps(bad)


# -------------------------------------------------------------------- model ---
class ModelTest(unittest.TestCase):
    def test_extract_last_block(self):
        content = "first\n```bash\necho a\n```\nthen\n```bash\necho b\n```"
        self.assertEqual(extract_action(content), "echo b")

    def test_no_block_is_format_error(self):
        with self.assertRaises(FormatError):
            extract_action("no code here")

    def test_replay_format_error_step(self):
        m = ReplayModel(steps=[{"format_error": "need a tool call"}])
        with self.assertRaises(FormatError):
            m.query([])

    def test_replay_validates_on_construction(self):
        with self.assertRaises(ConfigError):
            ReplayModel(steps=[{"reasoning": "no command"}])


# ------------------------------------------------------------- interactive ---
class FakeEnv:
    """A fast, hermetic environment mimicking the submit protocol (no bashy)."""

    def __init__(self):
        self.calls: list[str] = []

    def get_template_vars(self, **kwargs):
        return {}

    def execute(self, action, cwd="", **kwargs):
        cmd = action.get("command", "")
        self.calls.append(cmd)
        first = cmd.strip().splitlines()[0].strip() if cmd.strip() else ""
        if first == "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT":
            raise Submitted({"role": "exit", "content": "", "extra": {"exit_status": "Submitted", "submission": ""}})
        return {"output": cmd + "\n", "returncode": 0, "exception_info": ""}

    def serialize(self):
        return {"info": {"config": {"environment": {}, "environment_type": "fake"}}}


def _agent(steps, *, mode, responses, interactive=True, confirm_exit=True, whitelist=None):
    model = ReplayModel(steps=steps)
    config = InteractiveAgentConfig(mode=mode, confirm_exit=confirm_exit,
                                    whitelist_actions=whitelist or [])
    prompter = ScriptedPrompter(responses, interactive=interactive)
    return InteractiveAgent(model, FakeEnv(), config, prompter=prompter)


class InteractiveTest(unittest.TestCase):
    def test_confirm_approve_then_submit(self):
        # Approve the work action, then approve the submit action (both prompt in
        # confirm mode). confirm_exit off so submit terminates cleanly.
        a = _agent(
            [{"command": "echo work"}, {"command": "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"}],
            mode="confirm", responses=["", ""], confirm_exit=False,
        )
        result = a.run(task="t")
        self.assertEqual(result["exit_status"], "Submitted")
        self.assertEqual(a.env.calls, ["echo work", "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"])

    def test_confirm_reject(self):
        # Reject the first action (a comment); the model requeries and submits
        # (the submit is then approved).
        a = _agent(
            [{"command": "rm -rf /"}, {"command": "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"}],
            mode="confirm", responses=["do not do that", ""], confirm_exit=False,
        )
        result = a.run(task="t")
        self.assertEqual(result["exit_status"], "Submitted")
        self.assertEqual(a.env.calls, ["echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"])  # rejected one never ran

    def test_slash_y_switches_to_yolo(self):
        # /y at the confirm prompt switches mode; the action then runs without asking again.
        a = _agent(
            [{"command": "echo one"}, {"command": "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"}],
            mode="confirm", responses=["/y"],  # switch to yolo; no more prompts needed
            confirm_exit=False,
        )
        result = a.run(task="t")
        self.assertEqual(result["exit_status"], "Submitted")
        self.assertEqual(a.config.mode, "yolo")
        self.assertEqual(a.env.calls, ["echo one", "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"])

    def test_human_mode_types_command(self):
        a = _agent([], mode="human",
                   responses=["echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"], confirm_exit=False)
        result = a.run(task="t")
        self.assertEqual(result["exit_status"], "Submitted")

    def test_help_then_continue(self):
        # /h prints help and re-prompts; then approve, then submit runs.
        a = _agent(
            [{"command": "echo x"}, {"command": "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"}],
            mode="confirm", responses=["/h", "", ""], confirm_exit=False,
        )
        result = a.run(task="t")
        self.assertEqual(result["exit_status"], "Submitted")
        self.assertTrue(any("Current mode" in n for n in a.prompter.notices))

    def test_whitelist_skips_confirmation(self):
        a = _agent(
            [{"command": "echo safe"}, {"command": "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"}],
            mode="confirm", responses=[], whitelist=[r"echo ", r"echo COMPLETE"], confirm_exit=False,
        )
        result = a.run(task="t")
        self.assertEqual(result["exit_status"], "Submitted")  # never prompted (responses empty)

    def test_confirm_non_interactive_fails_closed(self):
        a = _agent([{"command": "echo x"}], mode="confirm", responses=[], interactive=False)
        with self.assertRaises(NonInteractiveApproval):
            a.run(task="t")

    def test_human_non_interactive_fails_closed(self):
        a = _agent([], mode="human", responses=[], interactive=False)
        with self.assertRaises(NonInteractiveApproval):
            a.run(task="t")

    def test_confirm_exit_new_task_continues(self):
        # On submit, user gives a new task -> UserNewTask interruption -> loop continues
        # -> next model step submits (this time quit).
        a = _agent(
            [{"command": "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"},
             {"command": "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"}],
            mode="yolo", responses=["a new task", ""], confirm_exit=True,
        )
        result = a.run(task="t")
        self.assertEqual(result["exit_status"], "Submitted")


# ---------------------------------------------------------------------- cli ---
class CliBuildSpecTest(unittest.TestCase):
    def _args(self, argv):
        return _build_parser().parse_args(argv)

    def test_unsupported_model_class(self):
        with self.assertRaises(UsageError):
            build_spec(self._args(["--model-class", "bogus", "-t", "x"]))

    def test_unsupported_agent_class(self):
        with self.assertRaises(UsageError):
            build_spec(self._args(["--agent-class", "wizard", "-t", "x"]))

    def test_unsupported_environment_class(self):
        with self.assertRaises(UsageError):
            build_spec(self._args(["--environment-class", "docker", "-t", "x"]))

    def test_supported_classes_ok(self):
        spec = build_spec(self._args(["--model-class", "replay", "--agent-class", "interactive",
                                      "--environment-class", "local", "-t", "x",
                                      "--replay", str(SCENARIOS / "noop-steps.json")]))
        self.assertEqual(spec["task"], "x")

    def test_bad_budget_via_cli(self):
        with self.assertRaises(ConfigError):
            build_spec(self._args(["-t", "x", "-l", "nan"]))


# --------------------------------------------------------------- trajectory ---
class TrajectoryTest(unittest.TestCase):
    @unittest.skipUnless(HAVE_BASHY, "requires a Bashy executor")
    def test_shape(self):
        spec = json.loads((SCENARIOS / "submit-success.json").read_text())
        traj = run_scenario(spec)["agent"].serialize()
        self.assertEqual(traj["trajectory_format"], TRAJECTORY_FORMAT)
        self.assertEqual(traj["info"]["exit_status"], "Submitted")
        self.assertEqual([m["role"] for m in traj["messages"][:2]], ["system", "user"])
        env = normalized_envelope(spec, run_scenario(spec))
        self.assertEqual(env["upstream_version"], UPSTREAM_VERSION)


if __name__ == "__main__":
    unittest.main()
