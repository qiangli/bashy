"""Flags-first `mini` CLI — the single local loop both entrypoints run.

Original to this bounded example (cf. upstream `minisweagent/run/mini.py`, a
typer app reaching a live provider + interactive UX). `main.bpp` (the shell
entrypoint) and the ycode-gated YAML bridge both exec this module, so the two
paths run the SAME loop.

Surface (flags-first, no subcommands — mirroring upstream `mini`):
  -t/--task, -c/--config (repeatable), -m/--model, -y/--yolo, -l/--cost-limit,
  -o/--output, --model-class, --agent-class, --environment-class,
  --exit-immediately.

Bounded example additions (documented in README):
  --scenario FILE   load a full offline spec (task/config/model steps/mode/env)
  --replay FILE     load scripted replay steps (JSON list or {steps,...})
  --emit-envelope   print the deterministic normalized result to stdout
  --step-limit N    expose the step budget on the CLI
  --base-url / --api-key   OpenAI-compatible endpoint (loopback in tests)

Exit codes: 0 submitted; 1 non-submit terminal (limits/format); 2 usage/
unsupported class; 3 config error; 4 fail-closed (no TTY for task/confirm/human);
130 user interruption.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from .config import ConfigError, load_config, validate_budget
from .exceptions import NonInteractiveApproval
from .model import LiveModelError, ReplayExhausted
from .prompter import Prompter
from .runner import build_agent, normalized_envelope, run_agent

# Accepted advanced-class values; anything else fails explicitly (exit 2).
_MODEL_CLASSES = {"", "replay", "openai", "litellm"}
_AGENT_CLASSES = {"", "interactive", "default", "minisweagent.agents.interactive.InteractiveAgent"}
_ENV_CLASSES = {"", "local", "bashy", "minisweagent.environments.local.LocalEnvironment"}

EXIT_SUBMITTED = 0
EXIT_TERMINAL = 1
EXIT_USAGE = 2
EXIT_CONFIG = 3
EXIT_FAILCLOSED = 4
EXIT_INTERRUPT = 130


class UsageError(Exception):
    pass


def _build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="mini",
        add_help=True,
        description="Bounded, self-contained mini-SWE-agent loop on the Bashy execution boundary.",
    )
    p.add_argument("-t", "--task", default=None, help="Task/problem statement to solve")
    p.add_argument("-c", "--config", action="append", default=[], metavar="SPEC",
                   help="JSON config file or dotted key=value (repeatable, recursively merged)")
    p.add_argument("-m", "--model", default=None, help="Model name")
    p.add_argument("-y", "--yolo", action="store_true", help="Run without confirmation")
    p.add_argument("-l", "--cost-limit", default=None, help="Cost limit; 0 disables")
    p.add_argument("--step-limit", default=None, help="Step limit; 0 disables")
    p.add_argument("-o", "--output", default=None, help="Trajectory output file")
    p.add_argument("--model-class", default="", help="Model class (replay|openai)")
    p.add_argument("--agent-class", default="", help="Agent class (interactive|default)")
    p.add_argument("--environment-class", default="", help="Environment class (local|bashy)")
    p.add_argument("--exit-immediately", action="store_true",
                   help="Do not prompt on finish (confirm_exit=false)")
    p.add_argument("--mode", default=None, choices=["confirm", "yolo", "human"],
                   help="Interaction mode (default confirm; -y forces yolo)")
    p.add_argument("--scenario", default=None, help="Load a full offline spec JSON")
    p.add_argument("--replay", default=None, help="Load scripted replay steps JSON")
    p.add_argument("--base-url", default=None, help="OpenAI-compatible base URL")
    p.add_argument("--api-key", default=None, help="OpenAI-compatible API key")
    p.add_argument("--emit-envelope", action="store_true",
                   help="Print the normalized result envelope to stdout")
    return p


def _load_json_file(path: str) -> dict:
    try:
        data = json.loads(Path(path).read_text())
    except FileNotFoundError as e:
        raise ConfigError(f"file not found: {path}") from e
    except json.JSONDecodeError as e:
        raise ConfigError(f"malformed JSON in {path!r}: {e}") from e
    return data


def build_spec(args) -> dict:
    """Assemble a normalized runner spec from parsed args + config, or raise."""
    if args.model_class not in _MODEL_CLASSES:
        raise UsageError(f"unsupported --model-class {args.model_class!r} (supported: replay, openai)")
    if args.agent_class not in _AGENT_CLASSES:
        raise UsageError(f"unsupported --agent-class {args.agent_class!r} (supported: interactive, default)")
    if args.environment_class not in _ENV_CLASSES:
        raise UsageError(f"unsupported --environment-class {args.environment_class!r} (supported: local, bashy)")

    spec: dict = {}
    if args.scenario:
        spec = _load_json_file(args.scenario)
        if not isinstance(spec, dict):
            raise ConfigError("scenario file must be a JSON object")
    spec.setdefault("config", {})
    spec.setdefault("model", {})
    spec.setdefault("environment", {})

    # -c config specs merge under config/model/environment sections if present.
    if args.config:
        merged = load_config(args.config)
        for section in ("config", "model", "environment"):
            if isinstance(merged.get(section), dict):
                spec[section].update(merged.pop(section))
        spec["config"].update(merged)  # remaining top-level keys treated as agent config

    # Model selection.
    model = spec["model"]
    if args.replay:
        replay = _load_json_file(args.replay)
        steps = replay["steps"] if isinstance(replay, dict) and "steps" in replay else replay
        model["class"] = "replay"
        model["steps"] = steps
        if isinstance(replay, dict):
            model.setdefault("model_name", replay.get("model_name", "replay/deterministic"))
            if "cost_per_call" in replay:
                model["cost_per_call"] = replay["cost_per_call"]
    if args.model_class in ("openai", "litellm"):
        model["class"] = "openai"
    elif args.model_class == "replay":
        model["class"] = "replay"
    if "class" not in model:
        # No explicit class: replay if steps were provided, else live openai.
        model["class"] = "replay" if model.get("steps") else "openai"
    if args.model:
        model["model_name"] = args.model
    if args.base_url:
        model["base_url"] = args.base_url
    if args.api_key is not None:
        model["api_key"] = args.api_key

    # Mode / confirmation.
    if args.mode:
        spec["mode"] = args.mode
    if args.yolo:
        spec["mode"] = "yolo"
    spec.setdefault("mode", "confirm")
    if args.exit_immediately:
        spec["config"]["confirm_exit"] = False

    # Budget (validated).
    if args.cost_limit is not None:
        spec["config"]["cost_limit"] = args.cost_limit
    if args.step_limit is not None:
        spec["config"]["step_limit"] = args.step_limit
    validate_budget(
        cost_limit=spec["config"].get("cost_limit", 3.0),
        step_limit=spec["config"].get("step_limit", 0),
    )

    if args.task is not None:
        spec["task"] = args.task
    return spec


def _persist(agent, output: str | None) -> None:
    if not output:
        return
    path = Path(output)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(agent.serialize(), indent=2, sort_keys=True))


def _exit_code(exit_status: str) -> int:
    if exit_status == "Submitted":
        return EXIT_SUBMITTED
    if exit_status == "UserInterruption":
        return EXIT_INTERRUPT
    return EXIT_TERMINAL


def main(argv: list[str] | None = None) -> int:
    parser = _build_parser()
    args = parser.parse_args(argv)
    prompter = Prompter()

    try:
        spec = build_spec(args)
    except UsageError as e:
        sys.stderr.write(f"error: {e}\n")
        return EXIT_USAGE
    except ConfigError as e:
        sys.stderr.write(f"error: {e}\n")
        return EXIT_CONFIG

    # Task prompting: fail closed if none and no terminal.
    if not spec.get("task"):
        if not prompter.is_interactive():
            sys.stderr.write("error: no --task and no interactive terminal to prompt (fail-closed)\n")
            return EXIT_FAILCLOSED
        try:
            spec["task"] = prompter.prompt("What do you want to do?\n> ").strip()
        except (EOFError, KeyboardInterrupt):
            sys.stderr.write("error: no task provided\n")
            return EXIT_FAILCLOSED
        if not spec["task"]:
            sys.stderr.write("error: empty task\n")
            return EXIT_FAILCLOSED

    import shutil
    import tempfile
    scratch = tempfile.mkdtemp(prefix="mswea-cli-")
    agent = None
    try:
        try:
            agent = build_agent(spec, cwd=scratch, prompter=prompter)
            run = run_agent(agent, spec["task"])
        except NonInteractiveApproval as e:
            if agent is not None:
                _persist(agent, args.output)  # trajectory persisted on fail-closed
            sys.stderr.write(f"error: {e}\n")
            return EXIT_FAILCLOSED
        except (ConfigError, ValueError) as e:
            sys.stderr.write(f"error: {e}\n")
            return EXIT_CONFIG
        except (ReplayExhausted, LiveModelError) as e:
            if agent is not None:
                _persist(agent, args.output)
            sys.stderr.write(f"error: {e}\n")
            return EXIT_TERMINAL

        _persist(agent, args.output)  # trajectory persisted on success and terminal outcomes
        envelope = normalized_envelope(spec, run)
        if args.emit_envelope:
            json.dump(envelope, sys.stdout, indent=2, sort_keys=True)
            sys.stdout.write("\n")
        else:
            status = envelope["exit_status"]
            sys.stderr.write(f"[mini] exit_status={status} steps={envelope['api_calls']} cost={envelope['cost']}\n")
            if status == "Submitted" and envelope["submission"]:
                sys.stdout.write(envelope["submission"])
                if not envelope["submission"].endswith("\n"):
                    sys.stdout.write("\n")
        return _exit_code(envelope["exit_status"])
    finally:
        shutil.rmtree(scratch, ignore_errors=True)


if __name__ == "__main__":
    raise SystemExit(main())
