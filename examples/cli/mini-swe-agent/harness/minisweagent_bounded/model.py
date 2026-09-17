"""Deterministic replay model — the offline stand-in for a live LM provider.

This module is ORIGINAL to the bounded example (not derived from upstream). The
upstream `mini` reaches a network model provider (litellm) whose replies drive
the loop; that cannot run offline or deterministically. The ReplayModel instead
replays a pre-recorded, scripted sequence of assistant steps (reasoning text +
one bash command each), so the real agent loop and environment run end-to-end
with no network and no model calls. Cost is fixed per call, so trajectories are
reproducible.

It implements the small model interface the agent depends on:
  format_message(role, content, extra) -> dict
  query(messages) -> assistant dict with extra.actions and extra.cost
  format_observation_messages(message, outputs, template_vars) -> list[dict]
  get_template_vars() -> dict
  serialize() -> dict
"""

from __future__ import annotations

import json
from typing import Any


class ReplayExhausted(RuntimeError):
    """Raised when the scripted steps run out before the loop terminated."""


class ReplayModel:
    def __init__(self, *, model_name: str = "replay/deterministic", cost_per_call: float = 0.0, steps=None):
        self.model_name = model_name
        self.cost_per_call = float(cost_per_call)
        self._steps = list(steps or [])
        self._cursor = 0
        self.n_calls = 0

    @staticmethod
    def format_message(role: str, content: str, extra: dict | None = None) -> dict:
        return {"role": role, "content": content, "extra": dict(extra or {})}

    def query(self, messages: list[dict], **kwargs) -> dict:
        if self._cursor >= len(self._steps):
            raise ReplayExhausted(
                f"replay model exhausted after {self._cursor} scripted step(s); "
                "the scenario never reached a submit or limit"
            )
        step = self._steps[self._cursor]
        self._cursor += 1
        self.n_calls += 1
        command = step["command"]
        reasoning = step.get("reasoning", "")
        return self.format_message(
            role="assistant",
            content=reasoning,
            extra={"actions": [{"command": command}], "cost": self.cost_per_call},
        )

    def format_observation_messages(self, message: dict, outputs: list[dict], template_vars: dict | None = None):
        msgs = []
        for output in outputs:
            observation = {"returncode": output.get("returncode"), "output": output.get("output", "")}
            if output.get("exception_info"):
                observation["exception_info"] = output["exception_info"]
            msgs.append(self.format_message(role="user", content=json.dumps(observation, sort_keys=True)))
        return msgs

    def get_template_vars(self, **kwargs) -> dict[str, Any]:
        return {"model_name": self.model_name}

    def serialize(self) -> dict:
        return {
            "info": {
                "config": {
                    "model": {"model_name": self.model_name, "cost_per_call": self.cost_per_call},
                    "model_type": f"{self.__class__.__module__}.{self.__class__.__name__}",
                }
            }
        }
