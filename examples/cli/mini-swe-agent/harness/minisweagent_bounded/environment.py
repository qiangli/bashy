"""Stateless Bashy execution environment with the mini-swe-agent submit protocol.

Materially derived from mini-swe-agent's `minisweagent/environments/local.py`
(MIT, (c) 2025 Kilian A. Lieret and Carlos E. Jimenez). Rewritten to depend only
on the Python standard library (pydantic removed; config is a plain object) and,
crucially, to run every action as a FRESH, STATELESS Bashy process via an
explicit argv `[BASHY_BIN, '-c', command]` with `shell=False` — never the host
`/bin/sh`, and with no host-shell fallback. Each action is independent: a `cd`
or an env change in one action does not leak into the next.

The substantive behavior is preserved: run a command, capture merged
stdout/stderr and the return code, and detect the explicit submit marker. On
timeout OR interruption the WHOLE owned process group is terminated and reaped,
so no child is orphaned. See ../README.md for the source pin and manifest.
"""

from __future__ import annotations

import os
import shutil
import signal
import subprocess
import time
from typing import Any

from .exceptions import Submitted

SUBMIT_MARKER = "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"

# Environment values upstream `mini.yaml` sets to keep tool output deterministic
# and non-interactive. Applied to every action's fresh process.
DEFAULT_ENV = {
    "PAGER": "cat",
    "MANPAGER": "cat",
    "PIP_PROGRESS_BAR": "off",
    "TQDM_DISABLE": "1",
}


class BashyNotFound(RuntimeError):
    """Raised when no Bashy executable can be resolved and no fallback is allowed."""


def resolve_bashy() -> str:
    """Resolve the Bashy executable. No host-shell fallback.

    Order: ``$BASHY_BIN`` (must be executable) -> ``bashy`` on ``PATH``. If
    neither resolves, raise — the bounded harness never silently runs actions
    through ``/bin/sh``.
    """
    override = os.environ.get("BASHY_BIN")
    if override:
        if os.path.isfile(override) and os.access(override, os.X_OK):
            return override
        raise BashyNotFound(f"BASHY_BIN is not an executable file: {override!r}")
    found = shutil.which("bashy")
    if found:
        return found
    raise BashyNotFound("no Bashy executable found: set BASHY_BIN or put `bashy` on PATH")


class LocalEnvironment:
    """Execute commands as independent, stateless Bashy processes."""

    def __init__(self, *, cwd: str = "", env: dict[str, str] | None = None, timeout: int = 30,
                 bashy_bin: str | None = None):
        self.cwd = cwd
        self.env = dict(env or {})
        self.timeout = timeout
        # Resolve eagerly so a misconfigured host fails before the first action.
        self.bashy_bin = bashy_bin or resolve_bashy()

    def execute(self, action: dict, cwd: str = "", *, timeout: int | None = None) -> dict[str, Any]:
        """Execute a command and return {output, returncode, exception_info}."""
        command = action.get("command", "")
        cwd = cwd or self.cwd or os.getcwd()
        run_env = {**os.environ, **DEFAULT_ENV, **self.env}
        try:
            result = _run(self.bashy_bin, command, cwd, run_env, timeout or self.timeout)
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
                    "environment": {"cwd": self.cwd, "timeout": self.timeout, "executor": self.bashy_bin},
                    "environment_type": f"{self.__class__.__module__}.{self.__class__.__name__}",
                }
            }
        }


def _reap_group(process: subprocess.Popen) -> None:
    """Terminate and reap the whole owned process group; no orphans (POSIX)."""
    if os.name != "posix":
        process.kill()
        return
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    # Give the group a brief moment to exit on SIGTERM, then SIGKILL the rest.
    deadline = time.time() + 2.0
    while time.time() < deadline:
        if process.poll() is not None:
            break
        time.sleep(0.02)
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass


def _run(bashy_bin: str, command: str, cwd: str, env: dict[str, str], timeout: int) -> subprocess.CompletedProcess[str]:
    """Run one command as a fresh stateless Bashy process.

    argv is explicit ``[bashy_bin, '-c', command]`` with ``shell=False`` — the
    host ``/bin/sh`` is never involved. On timeout OR a KeyboardInterrupt (Ctrl-C
    forwarded to this process while the child runs), the whole process group is
    terminated and reaped before re-raising.
    """
    process = subprocess.Popen(
        [bashy_bin, "-c", command],
        shell=False,
        text=True,
        cwd=cwd,
        env=env,
        encoding="utf-8",
        errors="replace",
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        start_new_session=os.name == "posix",
    )
    try:
        stdout, _ = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        _reap_group(process)
        stdout, _ = process.communicate()
        raise subprocess.TimeoutExpired(command, timeout, output=stdout)
    except KeyboardInterrupt:
        # A real Ctrl-C arrived mid-action: never leave the child group running.
        _reap_group(process)
        process.communicate()
        raise
    return subprocess.CompletedProcess(command, process.returncode, stdout=stdout)
