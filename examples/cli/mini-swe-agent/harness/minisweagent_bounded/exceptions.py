"""Agent-flow control exceptions.

Materially derived from mini-swe-agent's `minisweagent/exceptions.py`
(MIT, (c) 2025 Kilian A. Lieret and Carlos E. Jimenez), preserving the
agent-flow control hierarchy including `UserInterruption` (used by the
interactive loop). `NonInteractiveApproval` is original to this bounded example.
See ../README.md for the source pin and adaptation manifest.
"""


class InterruptAgentFlow(Exception):
    """Raised to interrupt the agent flow and add terminal messages."""

    def __init__(self, *messages: dict):
        self.messages = messages
        super().__init__()


class Submitted(InterruptAgentFlow):
    """Raised when the agent has completed its task."""


class LimitsExceeded(InterruptAgentFlow):
    """Raised when the agent has exceeded its cost or step limit."""


class TimeExceeded(LimitsExceeded):
    """Raised when the agent has exceeded its wall-clock time limit."""


class UserInterruption(InterruptAgentFlow):
    """Raised when the user interrupts the agent (e.g. Ctrl-C) or rejects an action."""


class FormatError(InterruptAgentFlow):
    """Raised when the model's output is not in the expected format."""


class NonInteractiveApproval(Exception):
    """Raised when a confirm/human decision is needed but no terminal is attached.

    The bounded harness fails closed rather than treating redirected/piped stdin
    as approval. This is a configuration/usage error, not an agent-flow signal, so
    it is intentionally NOT an InterruptAgentFlow.
    """
