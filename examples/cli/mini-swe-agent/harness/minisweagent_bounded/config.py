"""Config loading and strict validation for the bounded harness.

Original to this bounded example. Upstream `mini` merges YAML config documents,
filenames and `key=value` specs through jinja/pydantic; that machinery is out of
scope here. This module keeps a minimal, stdlib-only, reproducible subset:

  - `-c/--config` specs are JSON files (path ending in .json or containing a
    leading `{`) or dotted `key=value` overrides, recursively merged left-to-right.
  - budgets and replay/config payloads are validated STRICTLY: NaN, infinite and
    negative budgets are rejected, and malformed JSON/replay data raises a
    `ConfigError` rather than silently degrading.

Cost accounting is honest: cost is only ever `cost_per_call * n_calls` for the
replay model, or a provider-reported/operator-supplied per-call price for the
live model. When a provider's price is unknown the operator must pass
`cost_per_call=0` (or `--cost-limit 0` to disable the budget); the harness never
invents a price. See ../README.md.
"""

from __future__ import annotations

import json
import math
from pathlib import Path


class ConfigError(ValueError):
    """Raised on malformed config, replay data, or an invalid budget."""


def _merge(base: dict, overlay: dict) -> dict:
    for key, value in overlay.items():
        if isinstance(value, dict) and isinstance(base.get(key), dict):
            _merge(base[key], value)
        else:
            base[key] = value
    return base


def _coerce_scalar(text: str):
    """Coerce a `key=value` value into JSON scalar where possible, else str."""
    try:
        return json.loads(text)
    except (json.JSONDecodeError, ValueError):
        return text


def _apply_dotted(target: dict, dotted_key: str, value) -> None:
    parts = dotted_key.split(".")
    node = target
    for part in parts[:-1]:
        node = node.setdefault(part, {})
        if not isinstance(node, dict):
            raise ConfigError(f"config key {dotted_key!r} traverses a non-object")
    node[parts[-1]] = value


def load_config(specs: list[str]) -> dict:
    """Merge a list of `-c` specs (JSON files or dotted key=value) into one dict."""
    merged: dict = {}
    for spec in specs:
        if "=" in spec and not spec.strip().startswith("{"):
            key, _, raw = spec.partition("=")
            _apply_dotted(merged, key.strip(), _coerce_scalar(raw))
            continue
        try:
            if spec.strip().startswith("{"):
                data = json.loads(spec)
            else:
                path = Path(spec)
                if not path.is_file():
                    raise ConfigError(f"config file not found: {spec}")
                data = json.loads(path.read_text())
        except json.JSONDecodeError as e:
            raise ConfigError(f"malformed JSON config {spec!r}: {e}") from e
        if not isinstance(data, dict):
            raise ConfigError(f"config {spec!r} must be a JSON object")
        _merge(merged, data)
    return merged


def validate_budget(*, cost_limit: float, step_limit: int) -> tuple[float, int]:
    """Validate and normalize a budget. 0 disables a limit; negatives/NaN/inf reject."""
    try:
        cost = float(cost_limit)
    except (TypeError, ValueError) as e:
        raise ConfigError(f"cost limit is not a number: {cost_limit!r}") from e
    if math.isnan(cost) or math.isinf(cost):
        raise ConfigError(f"cost limit must be finite, got {cost_limit!r}")
    if cost < 0:
        raise ConfigError(f"cost limit must be >= 0 (0 disables), got {cost}")
    if isinstance(step_limit, bool):
        raise ConfigError(f"step limit is not an integer: {step_limit!r}")
    try:
        step_number = float(step_limit)
    except (TypeError, ValueError) as e:
        raise ConfigError(f"step limit is not an integer: {step_limit!r}") from e
    if math.isnan(step_number) or math.isinf(step_number) or not step_number.is_integer():
        raise ConfigError(f"step limit is not an integer: {step_limit!r}")
    steps = int(step_number)
    if steps < 0:
        raise ConfigError(f"step limit must be >= 0 (0 disables), got {steps}")
    return cost, steps


def validate_cost_per_call(value) -> float:
    try:
        cost = float(value)
    except (TypeError, ValueError) as e:
        raise ConfigError(f"cost_per_call is not a number: {value!r}") from e
    if math.isnan(cost) or math.isinf(cost):
        raise ConfigError(f"cost_per_call must be finite, got {value!r}")
    if cost < 0:
        raise ConfigError(f"cost_per_call must be >= 0, got {cost}")
    return cost


def validate_replay_steps(steps) -> list[dict]:
    """Validate a scripted replay step list: each step is an object with a
    `command` or a `format_error`, and nothing else surprising."""
    if not isinstance(steps, list):
        raise ConfigError("replay steps must be a list")
    validated: list[dict] = []
    for i, step in enumerate(steps):
        if not isinstance(step, dict):
            raise ConfigError(f"replay step {i} must be an object")
        if "command" not in step and "format_error" not in step:
            raise ConfigError(f"replay step {i} needs a 'command' or 'format_error'")
        if "command" in step and not isinstance(step["command"], str):
            raise ConfigError(f"replay step {i} 'command' must be a string")
        validated.append(step)
    return validated
