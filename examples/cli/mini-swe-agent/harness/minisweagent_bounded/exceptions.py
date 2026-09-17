"""Agent-flow control exceptions.

Materially derived from mini-swe-agent's `minisweagent/exceptions.py`
(MIT, (c) 2025 Kilian A. Lieret and Carlos E. Jimenez). Trimmed to the subset
the bounded offline harness uses. See ../README.md for the source pin.
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


class FormatError(InterruptAgentFlow):
    """Raised when the model's output is not in the expected format."""
