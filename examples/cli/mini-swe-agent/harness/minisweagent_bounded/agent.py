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

`InteractiveAgent` (below) ports the necessary interactive subset — confirm /
yolo / human modes, the `/c /y /u /h /m` controls, Ctrl-C interruption, and
confirm-on-exit — with a stdlib prompter and fail-closed non-interactive
behavior.

Deliberately NOT ported (out of scope for the bounded example):
  - jinja2 templating: replaced by a minimal `{{ var }}` substitution
    (`_render_template`). Conditional/loop template syntax is unsupported.
  - pydantic config models: replaced by plain dataclasses.
  - rich/prompt_toolkit console decoration and logging integrations.
"""

from __future__ import annotations

import re
import time
import traceback
from dataclasses import dataclass, field
from typing import Any

from .exceptions import (
    FormatError,
    InterruptAgentFlow,
    LimitsExceeded,
    NonInteractiveApproval,
    Submitted,
    TimeExceeded,
    UserInterruption,
)
from .prompter import Prompter

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


@dataclass
class InteractiveAgentConfig(AgentConfig):
    mode: str = "confirm"  # "human" | "confirm" | "yolo"
    whitelist_actions: list[str] = field(default_factory=list)
    confirm_exit: bool = True


class InteractiveAgent(DefaultAgent):
    """Puts the user in the loop: confirm/yolo/human modes + Ctrl-C interruption.

    Materially derived from mini-swe-agent's `agents/interactive.py`. The rich /
    prompt_toolkit UX is replaced by the stdlib `Prompter`; the substantive
    control flow (mode switching via `/c /y /u`, `/h` help, `/m` multiline,
    confirmation-before-execute, confirm-on-exit, and interrupt handling) is
    preserved. Confirm/human decisions with no attached terminal FAIL CLOSED
    (`NonInteractiveApproval`): piped stdin is never treated as approval.
    """

    _MODE_COMMANDS = {"/u": "human", "/c": "confirm", "/y": "yolo"}

    def __init__(self, model, env, config: InteractiveAgentConfig, *, prompter: Prompter | None = None):
        super().__init__(model=model, env=env, config=config)
        self.prompter = prompter or Prompter()

    # --- prompting + slash commands ----------------------------------------
    def _prompt_and_handle_slash_commands(self, prompt: str) -> str:
        response = self.prompter.prompt(prompt)
        if response == "/m":
            return self.prompter.prompt_multiline("Multiline comment")
        if response == "/h":
            self.prompter.notify(
                f"Current mode: {self.config.mode}\n"
                "/y switch to yolo (execute LM commands without confirmation)\n"
                "/c switch to confirm (confirm before executing LM commands)\n"
                "/u switch to human (execute commands you type)\n"
                "/m enter a multiline comment"
            )
            return self._prompt_and_handle_slash_commands(prompt)
        if response in self._MODE_COMMANDS:
            if self.config.mode == self._MODE_COMMANDS[response]:
                self.prompter.notify(f"Already in {self.config.mode} mode.")
                return self._prompt_and_handle_slash_commands(prompt)
            self.config.mode = self._MODE_COMMANDS[response]
            self.prompter.notify(f"Switched to {self.config.mode} mode.")
            return response
        return response

    def _require_interactive(self, what: str) -> None:
        if not self.prompter.is_interactive():
            raise NonInteractiveApproval(
                f"{what} requires an interactive terminal; refusing to proceed non-interactively (fail-closed)"
            )

    def _interrupt(self, content: str, *, itype: str = "UserInterruption"):
        raise UserInterruption({"role": "user", "content": content, "extra": {"interrupt_type": itype}})

    # --- query: human mode types the command --------------------------------
    def query(self) -> dict:
        if self.config.mode == "human":
            self._require_interactive("human mode")
            command = self._prompt_and_handle_slash_commands("> ")
            if command not in ("/y", "/c"):
                msg = {
                    "role": "user",
                    "content": f"User command:\n```bash\n{command}\n```",
                    "extra": {"actions": [{"command": command}], "cost": 0.0},
                }
                self.n_calls += 1
                self.add_messages(msg)
                return msg
        return super().query()

    # --- step: catch a real Ctrl-C -----------------------------------------
    def step(self) -> list[dict]:
        try:
            return super().step()
        except KeyboardInterrupt:
            if not self.prompter.is_interactive():
                # No terminal to ask what to do: stop cleanly, trajectory saved.
                self.add_messages(
                    {"role": "user", "content": "Interrupted by user (non-interactive).",
                     "extra": {"interrupt_type": "UserInterruption"}},
                    {"role": "exit", "content": "UserInterruption",
                     "extra": {"exit_status": "UserInterruption", "submission": ""}},
                )
                return []
            try:
                comment = self._prompt_and_handle_slash_commands(
                    "\nInterrupted. Type a comment/command (/h for commands, empty to continue)\n> "
                ).strip()
            except (EOFError, KeyboardInterrupt):
                # Second Ctrl-C / EOF at the prompt: hard quit, trajectory saved.
                self.add_messages(
                    {"role": "exit", "content": "UserInterruption",
                     "extra": {"exit_status": "UserInterruption", "submission": ""}},
                )
                return []
            if not comment or comment in self._MODE_COMMANDS:
                comment = "Temporary interruption caught."
            self._interrupt(f"Interrupted by user: {comment}")

    # --- execute: confirmation gating + confirm-on-exit --------------------
    def execute_actions(self, message: dict) -> list[dict]:
        actions = message.get("extra", {}).get("actions", [])
        commands = [a["command"] for a in actions]
        outputs: list[dict] = []
        try:
            self._ask_confirmation_or_interrupt(commands)
            for action in actions:
                outputs.append(self.env.execute(action))
        except Submitted as e:
            self._check_for_new_task_or_submit(e)
        finally:
            result = self.add_messages(
                *self.model.format_observation_messages(message, outputs, self.get_template_vars())
            )
        return result

    def _should_ask_confirmation(self, action: str) -> bool:
        return self.config.mode == "confirm" and not any(re.match(r, action) for r in self.config.whitelist_actions)

    def _ask_confirmation_or_interrupt(self, commands: list[str]) -> None:
        if not any(self._should_ask_confirmation(c) for c in commands):
            return
        self._require_interactive("confirm mode")
        response = self._prompt_and_handle_slash_commands(
            f"Execute {len(commands)} action(s)? Enter to confirm, type a comment to reject, /h for commands\n> "
        ).strip()
        if response in ("", "/y"):
            return  # confirmed
        if response == "/u":
            self._interrupt("Commands not executed. Switching to human mode.", itype="UserRejection")
        self._interrupt(
            f"Commands not executed. The user rejected your commands with: {response}", itype="UserRejection"
        )

    def _check_for_new_task_or_submit(self, e: Submitted):
        if self.config.confirm_exit and self.prompter.is_interactive():
            response = self._prompt_and_handle_slash_commands(
                "Agent wants to finish. Type a new task or Enter to quit (/h for commands)\n> "
            ).strip()
            if response == "/u":
                self._interrupt("Switched to human mode.")
            if response in self._MODE_COMMANDS:
                return self._check_for_new_task_or_submit(e)
            if response:
                self._interrupt(f"The user added a new task: {response}", itype="UserNewTask")
        raise e
