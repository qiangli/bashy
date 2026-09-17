"""User-prompting abstraction for the interactive loop (stdlib only).

Original to this bounded example. Upstream `mini` uses rich + prompt_toolkit for
its console/prompt UX; that is heavy and untestable offline, so it is out of
scope. This minimal `Prompter` reads single or multi-line input from stdin and
writes prompts/notices to stderr (keeping stdout reserved for the normalized
result envelope). It reports whether a real terminal is attached, which the
agent uses to FAIL CLOSED for confirm/human decisions when there is no TTY —
piped stdin is never treated as approval.

Tests inject `ScriptedPrompter` for deterministic, offline coverage.
"""

from __future__ import annotations

import sys


class Prompter:
    """Line/multiline prompter over stdin/stderr with TTY detection."""

    def __init__(self, *, in_stream=None, out_stream=None):
        self._in = in_stream if in_stream is not None else sys.stdin
        self._out = out_stream if out_stream is not None else sys.stderr

    def is_interactive(self) -> bool:
        try:
            return bool(self._in) and self._in.isatty()
        except (ValueError, OSError):
            return False

    def notify(self, message: str) -> None:
        self._out.write(message + "\n")
        self._out.flush()

    def prompt(self, message: str) -> str:
        self._out.write(message)
        self._out.flush()
        line = self._in.readline()
        if line == "":  # EOF
            raise EOFError("no input available")
        return line.rstrip("\n")

    def prompt_multiline(self, message: str) -> str:
        """Read lines until a lone '.' (or EOF). Mirrors a simple multiline mode."""
        self._out.write(message + " (end with a single '.' on its own line)\n")
        self._out.flush()
        lines: list[str] = []
        while True:
            line = self._in.readline()
            if line == "":
                break
            if line.rstrip("\n") == ".":
                break
            lines.append(line.rstrip("\n"))
        return "\n".join(lines)


class ScriptedPrompter(Prompter):
    """A deterministic prompter that returns queued responses (for tests)."""

    def __init__(self, responses, *, interactive: bool = True):
        self._responses = list(responses)
        self._interactive = interactive
        self.notices: list[str] = []

    def is_interactive(self) -> bool:
        return self._interactive

    def notify(self, message: str) -> None:
        self.notices.append(message)

    def prompt(self, message: str) -> str:
        if not self._responses:
            raise EOFError("scripted prompter exhausted")
        return self._responses.pop(0)

    def prompt_multiline(self, message: str) -> str:
        return self.prompt(message)
