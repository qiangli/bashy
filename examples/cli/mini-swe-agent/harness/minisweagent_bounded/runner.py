"""Offline scenario driver — the deterministic entrypoint for the bounded harness.

This module is ORIGINAL to the bounded example (upstream's entrypoint is
`minisweagent/run/mini.py`, a typer app that reaches a live model provider and
an interactive UX — neither can run offline or deterministically). The runner
loads a self-contained scenario document (task, prompt templates, resource
limits, a scripted replay model, and environment settings), drives the real
`DefaultAgent` step loop against a real `LocalEnvironment` and the `ReplayModel`,
and emits a NORMALIZED result envelope on stdout.

The envelope is intentionally free of timestamps, absolute paths, and host
identity so a future benchmark harness can compare runs byte-for-byte. It is
`replay-only`: no network and no model provider is contacted. Substantive online
inference happens only in the installed upstream `mini` (reached via
../main.bpp), never here.

Usage:
    python -m minisweagent_bounded.runner SCENARIO.json
    python -m minisweagent_bounded.runner --trajectory SCENARIO.json   # full traj
"""

from __future__ import annotations

import argparse
import json
import sys
import tempfile
from pathlib import Path

from . import BOUNDED_VERSION, TRAJECTORY_FORMAT, UPSTREAM_VERSION
from .agent import AgentConfig, DefaultAgent
from .environment import LocalEnvironment
from .exceptions import Submitted
from .model import ReplayModel

RESULT_SCHEMA = "mini-swe-agent-bounded-result-v1"


def _build_config(spec: dict) -> AgentConfig:
    cfg = spec.get("config", {})
    defaults = AgentConfig()
    return AgentConfig(
        system_template=cfg.get("system_template", defaults.system_template),
        instance_template=cfg.get("instance_template", defaults.instance_template),
        step_limit=int(cfg.get("step_limit", defaults.step_limit)),
        cost_limit=float(cfg.get("cost_limit", defaults.cost_limit)),
        wall_time_limit_seconds=int(cfg.get("wall_time_limit_seconds", defaults.wall_time_limit_seconds)),
        max_consecutive_format_errors=int(
            cfg.get("max_consecutive_format_errors", defaults.max_consecutive_format_errors)
        ),
    )


def run_scenario(spec: dict, *, cwd: str | None = None) -> dict:
    """Run one scenario end-to-end and return {result, agent, events}.

    When `cwd` is not given, each run executes in a private, throwaway working
    directory so shell actions never touch the caller's tree and stay
    reproducible.
    """
    if cwd is None:
        with tempfile.TemporaryDirectory(prefix="mswea-bounded-") as scratch:
            return run_scenario(spec, cwd=scratch)

    model_spec = spec.get("model", {})
    env_spec = spec.get("environment", {})

    model = ReplayModel(
        model_name=model_spec.get("model_name", "replay/deterministic"),
        cost_per_call=float(model_spec.get("cost_per_call", 0.0)),
        steps=model_spec.get("steps", []),
    )
    env = LocalEnvironment(cwd=cwd, timeout=int(env_spec.get("timeout", 30)))
    agent = DefaultAgent(model=model, env=env, config=_build_config(spec))

    # Record every executed action's command and returncode, including the
    # submit action (whose `execute` raises Submitted before returning).
    events: list[dict] = []
    original_execute = env.execute

    def recording_execute(action: dict, cwd: str = "", **kwargs):
        try:
            output = original_execute(action, cwd, **kwargs)
        except Submitted:
            events.append({"command": action.get("command", ""), "returncode": 0, "submitted": True})
            raise
        events.append({"command": action.get("command", ""), "returncode": output["returncode"]})
        return output

    env.execute = recording_execute  # type: ignore[method-assign]
    result = agent.run(task=spec.get("task", ""))
    return {"result": result, "agent": agent, "events": events}


def normalized_envelope(spec: dict, run: dict) -> dict:
    """Project a completed run into the deterministic benchmark envelope."""
    agent: DefaultAgent = run["agent"]
    result: dict = run["result"]
    return {
        "schema": RESULT_SCHEMA,
        "scenario": spec.get("name", ""),
        "upstream_version": UPSTREAM_VERSION,
        "bounded_version": BOUNDED_VERSION,
        "trajectory_format": TRAJECTORY_FORMAT,
        "exit_status": result.get("exit_status", ""),
        "submission": result.get("submission", ""),
        "model_name": agent.model.model_name,
        "api_calls": agent.n_calls,
        "cost": round(agent.cost, 6),
        "actions": run["events"],
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="minisweagent_bounded.runner", description=__doc__)
    parser.add_argument("scenario", type=Path, help="Path to a scenario JSON document.")
    parser.add_argument(
        "--trajectory",
        action="store_true",
        help="Emit the full linear trajectory instead of the normalized envelope.",
    )
    args = parser.parse_args(argv)

    spec = json.loads(args.scenario.read_text())
    run = run_scenario(spec)

    if args.trajectory:
        payload = run["agent"].serialize()
    else:
        payload = normalized_envelope(spec, run)
    json.dump(payload, sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
