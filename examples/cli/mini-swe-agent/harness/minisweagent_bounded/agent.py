"""The bounded DefaultAgent step loop.

Materially derived from mini-swe-agent's `minisweagent/agents/default.py`
(MIT, (c) 2025 Kilian A. Lieret and Carlos E. Jimenez). Rewritten to depend only
on the Python standard library while preserving the substantive control flow:

  system + instance prompt -> step loop -> query model for one bash command ->
  execute it in the environment -> feed the observation back -> repeat until an
  explicit submit marker, a resource limit, or repeated format errors.

The exit-status taxonomy (Submitted / LimitsExceeded / TimeExceeded /
RepeatedFormatError) and the linear trajectory shape ("mini-swe-agent-1.1") are
preserved verbatim. See ../README.md for the source pin and the adaptation
manifest.

Deliberately NOT ported (out of scope for the bounded offline harness):
  - jinja2 templating: replaced by a minimal `{{ var }}` substitution
    (`_render_template`). Conditional/loop template syntax is unsupported.
  - pydantic config models: replaced by a plain `AgentConfig` dataclass.
  - the interactive confirmation UX and logging integrations.
"""

from __future__ import annotations

import re
import time
import traceback
from dataclasses import dataclass, field
from typing import Any

from .exceptions import FormatError, InterruptAgentFlow, LimitsExceeded, TimeExceeded

_TEMPLATE_VAR = re.compile(r"{{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*}}")


def recursive_merge(*dicts: dict) -> dict:
    """Recursively merge dictionaries left-to-right (later values win).

    Mirrors mini-swe-agent's `utils.serialize.recursive_merge` for the nested
    trajectory dictionaries this harness builds.
    """
    merged: dict = {}
    for d in dicts:
        for key, value in d.items():
            if isinstance(value, dict) and isinstance(merged.get(key), dict):
                merged[key] = recursive_merge(merged[key], value)
            else:
                merged[key] = value
    return merged


@dataclass
class AgentConfig:
    """Bounded projection of upstream `AgentConfig` (stdlib dataclass, no pydantic)."""

    system_template: str = "You are a helpful assistant that can interact with a computer.\n"
    instance_template: str = "Please solve this issue: {{task}}\n"
    step_limit: int = 0
    """Maximum number of model calls the agent can make. 0 means no limit."""
    cost_limit: float = 3.0
    """Stop after exceeding (>=) this cost. 0 means no limit."""
    wall_time_limit_seconds: int = 0
    """Stop after this many seconds of wall-clock time. 0 means no limit."""
    max_consecutive_format_errors: int = 3
    """Exit after this many format errors in a row. 0 means no limit."""


@dataclass
class DefaultAgent:
    model: Any
    env: Any
    config: AgentConfig = field(default_factory=AgentConfig)

    def __post_init__(self) -> None:
        self.messages: list[dict] = []
        self.extra_template_vars: dict = {}
        self.cost = 0.0
        self.n_calls = 0
        self.n_consecutive_format_errors = 0
        self._start_time = time.time()

    # --- template rendering -------------------------------------------------
    def get_template_vars(self, **kwargs) -> dict:
        return recursive_merge(
            {
                "step_limit": self.config.step_limit,
                "cost_limit": self.config.cost_limit,
            },
            self.env.get_template_vars() if hasattr(self.env, "get_template_vars") else {},
            self.model.get_template_vars(),
            {
                "n_model_calls": self.n_calls,
                "model_cost": self.cost,
                "elapsed_seconds": int(time.time() - self._start_time),
            },
            self.extra_template_vars,
            kwargs,
        )

    def _render_template(self, template: str) -> str:
        """Minimal, strict `{{ var }}` substitution (no jinja2).

        Unlike upstream, conditionals and loops are unsupported; an undefined
        variable raises, mirroring jinja2's StrictUndefined.
        """
        template_vars = self.get_template_vars()

        def _sub(match: re.Match) -> str:
            name = match.group(1)
            if name not in template_vars:
                raise KeyError(f"undefined template variable: {name!r}")
            return str(template_vars[name])

        return _TEMPLATE_VAR.sub(_sub, template)

    # --- trajectory bookkeeping --------------------------------------------
    def add_messages(self, *messages: dict) -> list[dict]:
        self.messages.extend(messages)
        return list(messages)

    def handle_uncaught_exception(self, e: Exception) -> list[dict]:
        return self.add_messages(
            self.model.format_message(
                role="exit",
                content=str(e),
                extra={
                    "exit_status": type(e).__name__,
                    "submission": "",
                    "exception_str": str(e),
                    "traceback": traceback.format_exc(),
                },
            )
        )

    # --- the loop -----------------------------------------------------------
    def run(self, task: str = "", **kwargs) -> dict:
        """Run step() until the agent is finished.

        Returns the terminal message's `extra` dict (exit_status, submission).
        """
        self.extra_template_vars = recursive_merge(self.extra_template_vars, {"task": task, **kwargs})
        self.messages = []
        self.add_messages(
            self.model.format_message(role="system", content=self._render_template(self.config.system_template)),
            self.model.format_message(role="user", content=self._render_template(self.config.instance_template)),
        )
        while True:
            try:
                self.step()
                self.n_consecutive_format_errors = 0  # reset on any clean step
            except FormatError as e:
                # The call was billed before parsing failed, so query() never charged it.
                self.cost += e.messages[0].get("extra", {}).get("cost", 0.0)
                self.n_consecutive_format_errors += 1
                if 0 < self.config.max_consecutive_format_errors <= self.n_consecutive_format_errors:
                    self.add_messages(
                        *e.messages,
                        {
                            "role": "exit",
                            "content": "RepeatedFormatError",
                            "extra": {"exit_status": "RepeatedFormatError", "submission": ""},
                        },
                    )
                else:
                    self.add_messages(*e.messages)
            except InterruptAgentFlow as e:
                self.add_messages(*e.messages)
            except Exception as e:  # noqa: BLE001 - recorded, then re-raised
                self.handle_uncaught_exception(e)
                raise
            if self.messages[-1].get("role") == "exit":
                break
        return self.messages[-1].get("extra", {})

    def step(self) -> list[dict]:
        return self.execute_actions(self.query())

    def query(self) -> dict:
        if 0 < self.config.step_limit <= self.n_calls or 0 < self.config.cost_limit <= self.cost:
            raise LimitsExceeded(
                {"role": "exit", "content": "LimitsExceeded", "extra": {"exit_status": "LimitsExceeded", "submission": ""}}
            )
        if 0 < self.config.wall_time_limit_seconds <= int(time.time() - self._start_time):
            raise TimeExceeded(
                {"role": "exit", "content": "TimeExceeded", "extra": {"exit_status": "TimeExceeded", "submission": ""}}
            )
        self.n_calls += 1
        message = self.model.query(self.messages)
        self.cost += message.get("extra", {}).get("cost", 0.0)
        self.add_messages(message)
        return message

    def execute_actions(self, message: dict) -> list[dict]:
        outputs = [self.env.execute(action) for action in message.get("extra", {}).get("actions", [])]
        return self.add_messages(*self.model.format_observation_messages(message, outputs, self.get_template_vars()))

    # --- serialization ------------------------------------------------------
    def serialize(self, *extra_dicts) -> dict:
        last_message = self.messages[-1] if self.messages else {}
        last_extra = last_message.get("extra", {})
        agent_data = {
            "info": {
                "model_stats": {"instance_cost": self.cost, "api_calls": self.n_calls},
                "config": {
                    "agent": {
                        "system_template": self.config.system_template,
                        "instance_template": self.config.instance_template,
                        "step_limit": self.config.step_limit,
                        "cost_limit": self.config.cost_limit,
                        "wall_time_limit_seconds": self.config.wall_time_limit_seconds,
                        "max_consecutive_format_errors": self.config.max_consecutive_format_errors,
                    },
                    "agent_type": f"{self.__class__.__module__}.{self.__class__.__name__}",
                },
                "exit_status": last_extra.get("exit_status", ""),
                "submission": last_extra.get("submission", ""),
            },
            "messages": self.messages,
            "trajectory_format": "mini-swe-agent-1.1",
        }
        return recursive_merge(agent_data, self.model.serialize(), self.env.serialize(), *extra_dicts)
