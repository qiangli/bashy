"""Local execution environment with the mini-swe-agent submit protocol.

Materially derived from mini-swe-agent's `minisweagent/environments/local.py`
(MIT, (c) 2025 Kilian A. Lieret and Carlos E. Jimenez). Rewritten to depend only
on the Python standard library (pydantic removed; config is a plain object) while
preserving the substantive behavior: run a bash command, capture merged
stdout/stderr and the return code, and detect the explicit submit marker. See
../README.md for the source pin and the adaptation manifest.
"""

from __future__ import annotations

import os
import signal
import subprocess
from typing import Any

from .exceptions import Submitted

SUBMIT_MARKER = "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"


class LocalEnvironment:
    """Execute bash commands directly on the local machine, deterministically."""

    def __init__(self, *, cwd: str = "", env: dict[str, str] | None = None, timeout: int = 30):
        self.cwd = cwd
        self.env = dict(env or {})
        self.timeout = timeout

    def execute(self, action: dict, cwd: str = "", *, timeout: int | None = None) -> dict[str, Any]:
        """Execute a command and return {output, returncode, exception_info}."""
        command = action.get("command", "")
        cwd = cwd or self.cwd or os.getcwd()
        try:
            result = _run(command, cwd, os.environ | self.env, timeout or self.timeout)
            output = {"output": result.stdout, "returncode": result.returncode, "exception_info": ""}
        except Exception as e:  # noqa: BLE001 - surfaced to the agent as an observation
            raw_output = getattr(e, "output", None)
            raw_output = (
                raw_output.decode("utf-8", errors="replace") if isinstance(raw_output, bytes) else (raw_output or "")
            )
            output = {
                "output": raw_output,
                "returncode": -1,
                "exception_info": f"An error occurred while executing the command: {e}",
            }
        self._check_finished(output)
        return output

    def _check_finished(self, output: dict) -> None:
        """Raise Submitted if the command output signals task completion."""
        lines = output.get("output", "").lstrip().splitlines(keepends=True)
        if lines and lines[0].strip() == SUBMIT_MARKER and output["returncode"] == 0:
            submission = "".join(lines[1:])
            raise Submitted(
                {
                    "role": "exit",
                    "content": submission,
                    "extra": {"exit_status": "Submitted", "submission": submission},
                }
            )

    def serialize(self) -> dict:
        return {
            "info": {
                "config": {
                    "environment": {"cwd": self.cwd, "timeout": self.timeout},
                    "environment_type": f"{self.__class__.__module__}.{self.__class__.__name__}",
                }
            }
        }


def _run(command: str, cwd: str, env: dict[str, str], timeout: int) -> subprocess.CompletedProcess[str]:
    """Like subprocess.run, but kills the whole process group on timeout so no children are orphaned."""
    process = subprocess.Popen(
        command,
        shell=True,
        text=True,
        cwd=cwd,
        env=env,
        encoding="utf-8",
        errors="replace",
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        start_new_session=os.name == "posix",
    )
    try:
        stdout, _ = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL) if os.name == "posix" else process.kill()
        stdout, _ = process.communicate()
        raise subprocess.TimeoutExpired(command, timeout, output=stdout)
    return subprocess.CompletedProcess(command, process.returncode, stdout=stdout)
