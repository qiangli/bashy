"""Deterministic, offline tests for the bounded mini-swe-agent harness.

No network and no model provider is contacted; the ReplayModel scripts every
step. Run from the harness/ directory:

    python3 -m unittest discover -s tests
    # or
    python3 -m pytest tests
"""

from __future__ import annotations

import json
import unittest
from pathlib import Path

from minisweagent_bounded import TRAJECTORY_FORMAT, UPSTREAM_VERSION
from minisweagent_bounded.agent import AgentConfig, DefaultAgent
from minisweagent_bounded.environment import LocalEnvironment
from minisweagent_bounded.exceptions import Submitted
from minisweagent_bounded.model import ReplayExhausted, ReplayModel
from minisweagent_bounded.runner import normalized_envelope, run_scenario

FIXTURES = Path(__file__).resolve().parents[2] / "fixtures"
SCENARIOS = FIXTURES / "scenarios"
GOLDENS = FIXTURES / "goldens"

SCENARIO_NAMES = [
    "submit-success",
    "cost-limit",
    "step-limit",
    "repeated-format-error",
    "format-error-recovers",
]


class ScenarioGoldenTest(unittest.TestCase):
    """Each scenario's normalized envelope must equal its committed golden."""

    def test_scenarios_match_goldens(self):
        for name in SCENARIO_NAMES:
            with self.subTest(scenario=name):
                spec = json.loads((SCENARIOS / name).with_suffix(".json").read_text())
                run = run_scenario(spec)
                got = normalized_envelope(spec, run)
                want = json.loads((GOLDENS / name).with_suffix(".json").read_text())
                self.assertEqual(got, want)

    def test_run_is_deterministic(self):
        spec = json.loads((SCENARIOS / "submit-success.json").read_text())
        first = normalized_envelope(spec, run_scenario(spec))
        second = normalized_envelope(spec, run_scenario(spec))
        self.assertEqual(first, second)


class ExitTaxonomyTest(unittest.TestCase):
    """The four terminal exit statuses are each reachable deterministically."""

    def _run(self, name: str) -> dict:
        spec = json.loads((SCENARIOS / name).with_suffix(".json").read_text())
        return normalized_envelope(spec, run_scenario(spec))

    def test_submitted(self):
        env = self._run("submit-success")
        self.assertEqual(env["exit_status"], "Submitted")
        self.assertEqual(env["submission"], "done\n")
        self.assertTrue(env["actions"][-1]["submitted"])

    def test_cost_limit(self):
        env = self._run("cost-limit")
        self.assertEqual(env["exit_status"], "LimitsExceeded")
        self.assertEqual(env["cost"], 0.02)
        self.assertEqual(len(env["actions"]), 2)

    def test_step_limit(self):
        env = self._run("step-limit")
        self.assertEqual(env["exit_status"], "LimitsExceeded")
        self.assertEqual(env["api_calls"], 2)

    def test_repeated_format_error(self):
        env = self._run("repeated-format-error")
        self.assertEqual(env["exit_status"], "RepeatedFormatError")
        self.assertEqual(env["actions"], [])

    def test_format_error_counter_resets(self):
        env = self._run("format-error-recovers")
        self.assertEqual(env["exit_status"], "Submitted")
        self.assertEqual(env["submission"], "finished\n")


class SubmitProtocolTest(unittest.TestCase):
    """The LocalEnvironment submit marker follows the upstream contract."""

    def test_marker_first_line_and_rc_zero_submits(self):
        env = LocalEnvironment()
        with self.assertRaises(Submitted) as ctx:
            env.execute({"command": "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\necho payload"})
        self.assertEqual(ctx.exception.messages[0]["extra"]["submission"], "payload\n")

    def test_marker_not_first_line_does_not_submit(self):
        env = LocalEnvironment()
        out = env.execute({"command": "echo before\necho COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"})
        self.assertEqual(out["returncode"], 0)

    def test_nonzero_return_code_does_not_submit(self):
        env = LocalEnvironment()
        out = env.execute({"command": "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT; false"})
        self.assertEqual(out["returncode"], 1)

    def test_stateless_actions(self):
        """Each action runs in a fresh subshell: a cd in one does not leak."""
        env = LocalEnvironment()
        env.execute({"command": "cd /"})
        out = env.execute({"command": "pwd"})
        self.assertNotEqual(out["output"].strip(), "/")


class ReplayModelTest(unittest.TestCase):
    def test_exhaustion_raises(self):
        model = ReplayModel(steps=[{"command": "echo hi"}])
        agent = DefaultAgent(model=model, env=LocalEnvironment(), config=AgentConfig(cost_limit=0, step_limit=0))
        with self.assertRaises(ReplayExhausted):
            agent.run(task="never submits")


class TrajectoryShapeTest(unittest.TestCase):
    def test_trajectory_format_and_version(self):
        spec = json.loads((SCENARIOS / "submit-success.json").read_text())
        run = run_scenario(spec)
        traj = run["agent"].serialize()
        self.assertEqual(traj["trajectory_format"], TRAJECTORY_FORMAT)
        self.assertEqual(traj["info"]["exit_status"], "Submitted")
        self.assertEqual(traj["info"]["model_stats"]["api_calls"], 3)
        # First two messages are always system then user (instance).
        self.assertEqual([m["role"] for m in traj["messages"][:2]], ["system", "user"])
        env = normalized_envelope(spec, run)
        self.assertEqual(env["upstream_version"], UPSTREAM_VERSION)


if __name__ == "__main__":
    unittest.main()
