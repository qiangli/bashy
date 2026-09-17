"""Models: a deterministic offline replay model and a minimal live transport.

Original to this bounded example. Upstream `mini` reaches model providers through
litellm; that cannot run offline or deterministically and pulls a large
dependency. This module implements two stdlib-only models behind the small model
interface the agent depends on:

  - `ReplayModel`   — replays a pre-recorded, scripted sequence of assistant
    steps (reasoning + one bash command each, or a scripted format error). Fully
    deterministic and offline. Cost is `cost_per_call * n_calls`.
  - `LiveModel`     — a minimal OpenAI-compatible chat/completions transport over
    `urllib` (no third-party SDK). The action is the LAST fenced bash block in the
    assistant reply; a reply with no bash block raises `FormatError`, mirroring
    upstream. During verification it is pointed at a LOOPBACK fake provider, never
    a paid endpoint.

Cost accounting is honest (see config.py): the live model bills a per-call price
that the OPERATOR supplies (`cost_per_call`, default 0 = unknown). The harness
never invents a provider price.

The small model interface:
  format_message(role, content, extra) -> dict
  query(messages) -> assistant dict with extra.actions and extra.cost
  format_observation_messages(message, outputs, template_vars) -> list[dict]
  get_template_vars() -> dict
  serialize() -> dict
"""

from __future__ import annotations

import json
import os
import re
import urllib.error
import urllib.request
from typing import Any

from .config import validate_cost_per_call, validate_replay_steps
from .exceptions import FormatError

# Last fenced block, preferring an explicit ```bash / ```sh tag but accepting a
# bare ``` fence, matching upstream's "last code block is the action" rule.
_FENCE = re.compile(r"```(?:bash|sh)?\s*\n(.*?)```", re.DOTALL)


def extract_action(content: str) -> str:
    """Return the command in the LAST fenced block, or raise FormatError."""
    blocks = _FENCE.findall(content or "")
    if not blocks:
        raise FormatError(
            {"role": "assistant", "content": content, "extra": {"cost": 0.0}},
            {
                "role": "user",
                "content": (
                    "Your response contained no bash tool call. Every response must include exactly one "
                    "```bash\\n...\\n``` block."
                ),
            },
        )
    return blocks[-1].strip("\n")


class ReplayExhausted(RuntimeError):
    """Raised when the scripted steps run out before the loop terminated."""


def _format_message(role: str, content: str, extra: dict | None = None) -> dict:
    return {"role": role, "content": content, "extra": dict(extra or {})}


def _format_observation_messages(model, message: dict, outputs: list[dict], template_vars: dict | None = None):
    msgs = []
    for output in outputs:
        observation = {"returncode": output.get("returncode"), "output": _truncate(output.get("output", ""))}
        if output.get("exception_info"):
            observation["exception_info"] = output["exception_info"]
        msgs.append(model.format_message(role="user", content=json.dumps(observation, sort_keys=True)))
    return msgs


# Output truncation, mirroring upstream mini.yaml's 10k head/tail elision so a
# runaway command cannot blow up the trajectory or a live request.
_TRUNCATE_LIMIT = 10000
_TRUNCATE_KEEP = 5000


def _truncate(text: str) -> str:
    if len(text) <= _TRUNCATE_LIMIT:
        return text
    head, tail = text[:_TRUNCATE_KEEP], text[-_TRUNCATE_KEEP:]
    elided = len(text) - 2 * _TRUNCATE_KEEP
    return f"{head}\n[... {elided} characters elided ...]\n{tail}"


class ReplayModel:
    def __init__(self, *, model_name: str = "replay/deterministic", cost_per_call: float = 0.0, steps=None):
        self.model_name = model_name
        self.cost_per_call = validate_cost_per_call(cost_per_call)
        self._steps = validate_replay_steps(list(steps or []))
        self._cursor = 0
        self.n_calls = 0

    @staticmethod
    def format_message(role: str, content: str, extra: dict | None = None) -> dict:
        return _format_message(role, content, extra)

    def query(self, messages: list[dict], **kwargs) -> dict:
        if self._cursor >= len(self._steps):
            raise ReplayExhausted(
                f"replay model exhausted after {self._cursor} scripted step(s); "
                "the scenario never reached a submit or limit"
            )
        step = self._steps[self._cursor]
        self._cursor += 1
        self.n_calls += 1
        reasoning = step.get("reasoning", "")
        if "format_error" in step:
            assistant = self.format_message(role="assistant", content=reasoning, extra={"cost": self.cost_per_call})
            guidance = self.format_message(role="user", content=str(step["format_error"]))
            raise FormatError(assistant, guidance)
        return self.format_message(
            role="assistant",
            content=reasoning,
            extra={"actions": [{"command": step["command"]}], "cost": self.cost_per_call},
        )

    def format_observation_messages(self, message: dict, outputs: list[dict], template_vars: dict | None = None):
        return _format_observation_messages(self, message, outputs, template_vars)

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


class LiveModelError(RuntimeError):
    """Raised on a transport/protocol failure talking to the model provider."""


class LiveModel:
    """Minimal OpenAI-compatible chat/completions client (stdlib urllib)."""

    def __init__(
        self,
        *,
        model_name: str,
        base_url: str | None = None,
        api_key: str | None = None,
        cost_per_call: float = 0.0,
        timeout: float = 30.0,
    ):
        self.model_name = model_name
        self.base_url = (base_url or os.environ.get("OPENAI_BASE_URL") or "https://api.openai.com/v1").rstrip("/")
        self.api_key = api_key if api_key is not None else os.environ.get("OPENAI_API_KEY", "")
        self.cost_per_call = validate_cost_per_call(cost_per_call)
        self.timeout = timeout
        self.n_calls = 0

    @staticmethod
    def format_message(role: str, content: str, extra: dict | None = None) -> dict:
        return _format_message(role, content, extra)

    def _post(self, payload: dict) -> dict:
        data = json.dumps(payload).encode("utf-8")
        req = urllib.request.Request(f"{self.base_url}/chat/completions", data=data, method="POST")
        req.add_header("Content-Type", "application/json")
        if self.api_key:
            req.add_header("Authorization", f"Bearer {self.api_key}")
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except urllib.error.HTTPError as e:  # noqa: PERF203
            body = e.read().decode("utf-8", errors="replace")
            raise LiveModelError(f"provider returned HTTP {e.code}: {body}") from e
        except (urllib.error.URLError, TimeoutError, OSError) as e:
            raise LiveModelError(f"provider transport error: {e}") from e

    def query(self, messages: list[dict], **kwargs) -> dict:
        self.n_calls += 1
        wire = [
            {"role": m["role"], "content": m.get("content", "")}
            for m in messages
            if m.get("role") in ("system", "user", "assistant")
        ]
        response = self._post({"model": self.model_name, "messages": wire})
        try:
            content = response["choices"][0]["message"]["content"]
        except (KeyError, IndexError, TypeError) as e:
            raise LiveModelError(f"malformed provider response: {response!r}") from e
        try:
            command = extract_action(content)
        except FormatError:
            # The call was billed even though parsing failed; carry the cost so
            # the agent charges it (mirroring upstream).
            assistant = self.format_message(role="assistant", content=content, extra={"cost": self.cost_per_call})
            guidance = self.format_message(
                role="user",
                content=(
                    "Your response contained no bash tool call. Every response must include exactly one "
                    "```bash\\n...\\n``` block."
                ),
            )
            raise FormatError(assistant, guidance) from None
        return self.format_message(
            role="assistant",
            content=content,
            extra={"actions": [{"command": command}], "cost": self.cost_per_call},
        )

    def format_observation_messages(self, message: dict, outputs: list[dict], template_vars: dict | None = None):
        return _format_observation_messages(self, message, outputs, template_vars)

    def get_template_vars(self, **kwargs) -> dict[str, Any]:
        return {"model_name": self.model_name}

    def serialize(self) -> dict:
        return {
            "info": {
                "config": {
                    "model": {
                        "model_name": self.model_name,
                        "cost_per_call": self.cost_per_call,
                        "base_url": self.base_url,
                    },
                    "model_type": f"{self.__class__.__module__}.{self.__class__.__name__}",
                }
            }
        }
