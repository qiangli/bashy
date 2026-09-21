"""Shared harness core: build an agent from a spec, run it, normalize the result.

Original to this bounded example. This is the ONE loop both entrypoints use:
`main.bsh` (the shell entrypoint) and the ycode-gated YAML bridge both invoke
`cli.py`, which builds a normalized spec and calls `build_agent` + `run_agent`
here. Unit tests and golden generation call `run_scenario` directly.

A spec is a plain dict:
    {
      "name": str,                         # scenario label (optional)
      "task": str,
      "mode": "confirm"|"yolo"|"human",    # default "yolo" (deterministic/offline)
      "config": {system_template, instance_template, step_limit, cost_limit,
                 wall_time_limit_seconds, max_consecutive_format_errors,
                 confirm_exit, whitelist_actions},
      "model": {model_name, cost_per_call,
                replay?: [steps...],       # offline replay
                class?: "replay"|"openai", base_url?, api_key?},
      "environment": {timeout, cwd}
    }
The normalized envelope is deterministic (no timestamps/paths/host identity).
"""

from __future__ import annotations

import tempfile

from . import BOUNDED_VERSION, TRAJECTORY_FORMAT, UPSTREAM_VERSION
from .agent import InteractiveAgent, InteractiveAgentConfig
from .config import validate_budget, validate_replay_steps
from .environment import LocalEnvironment
from .exceptions import Submitted
from .model import LiveModel, ReplayModel
from .prompter import Prompter

RESULT_SCHEMA = "mini-swe-agent-bounded-result-v1"


def build_model(model_spec: dict):
    """Select and build the model. Replay by default; 'openai' for live transport."""
    kind = model_spec.get("class", "replay")
    name = model_spec.get("model_name", "replay/deterministic")
    cost_per_call = model_spec.get("cost_per_call", 0.0)
    if kind == "replay":
        steps = model_spec.get("steps", model_spec.get("replay", []))
        return ReplayModel(
            model_name=name,
            cost_per_call=cost_per_call,
            steps=validate_replay_steps(steps),
        )
    if kind == "openai":
        return LiveModel(
            model_name=name,
            base_url=model_spec.get("base_url"),
            api_key=model_spec.get("api_key"),
            cost_per_call=cost_per_call,
        )
    raise ValueError(f"unsupported model class: {kind!r} (supported: replay, openai)")


def build_config(spec: dict) -> InteractiveAgentConfig:
    cfg = spec.get("config", {})
    defaults = InteractiveAgentConfig()
    cost_limit, step_limit = validate_budget(
        cost_limit=cfg.get("cost_limit", defaults.cost_limit),
        step_limit=cfg.get("step_limit", defaults.step_limit),
    )
    return InteractiveAgentConfig(
        system_template=cfg.get("system_template", defaults.system_template),
        instance_template=cfg.get("instance_template", defaults.instance_template),
        step_limit=step_limit,
        cost_limit=cost_limit,
        wall_time_limit_seconds=int(cfg.get("wall_time_limit_seconds", defaults.wall_time_limit_seconds)),
        max_consecutive_format_errors=int(
            cfg.get("max_consecutive_format_errors", defaults.max_consecutive_format_errors)
        ),
        mode=spec.get("mode", "yolo"),
        whitelist_actions=list(cfg.get("whitelist_actions", [])),
        confirm_exit=bool(cfg.get("confirm_exit", defaults.confirm_exit)),
    )


def build_agent(spec: dict, *, cwd: str, prompter: Prompter | None = None):
    env_spec = spec.get("environment", {})
    model = build_model(spec.get("model", {}))
    env = LocalEnvironment(cwd=cwd, timeout=int(env_spec.get("timeout", 30)))
    config = build_config(spec)
    agent = InteractiveAgent(model, env, config, prompter=prompter)
    return agent


def run_agent(agent, task: str) -> dict:
    """Run one agent, recording each executed action's command + returncode."""
    events: list[dict] = []
    original_execute = agent.env.execute

    def recording_execute(action: dict, cwd: str = "", **kwargs):
        try:
            output = original_execute(action, cwd, **kwargs)
        except Submitted:
            events.append({"command": action.get("command", ""), "returncode": 0, "submitted": True})
            raise
        events.append({"command": action.get("command", ""), "returncode": output["returncode"]})
        return output

    agent.env.execute = recording_execute  # type: ignore[method-assign]
    result = agent.run(task=task)
    return {"result": result, "agent": agent, "events": events}


def run_scenario(spec: dict, *, cwd: str | None = None) -> dict:
    """Build + run a scenario in a private throwaway cwd (unless cwd is given)."""
    if cwd is None:
        with tempfile.TemporaryDirectory(prefix="mswea-bounded-") as scratch:
            return run_scenario(spec, cwd=scratch)
    agent = build_agent(spec, cwd=cwd)
    return run_agent(agent, spec.get("task", ""))


def normalized_envelope(spec: dict, run: dict) -> dict:
    agent = run["agent"]
    result = run["result"]
    return {
        "schema": RESULT_SCHEMA,
        "scenario": spec.get("name", ""),
        "upstream_version": UPSTREAM_VERSION,
        "bounded_version": BOUNDED_VERSION,
        "trajectory_format": TRAJECTORY_FORMAT,
        "mode": agent.config.mode,
        "exit_status": result.get("exit_status", ""),
        "submission": result.get("submission", ""),
        "model_name": agent.model.model_name,
        "api_calls": agent.n_calls,
        "cost": round(agent.cost, 6),
        "actions": run["events"],
    }
