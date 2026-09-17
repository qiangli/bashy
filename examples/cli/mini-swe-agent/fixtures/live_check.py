"""Live-transport fixture: the LiveModel against a loopback fake provider.

Starts the in-process fake OpenAI-compatible provider (fake_provider.py) and runs
the ACTUAL cli.py as a subprocess pointed at the loopback URL, so the real urllib
transport is exercised end-to-end with NO network egress and NO paid model call.
The scripted replies include a malformed (no-bash-block) response to exercise the
live FormatError -> guidance path before a clean submit.

Offline in the sense that matters: bound to 127.0.0.1, deterministic, free.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
from fake_provider import run_server  # noqa: E402

HARNESS = HERE.parent / "harness"

REPLIES = [
    "I'll look around first.",  # no bash block -> FormatError + guidance
    "Now a real command.\n```bash\necho recovered-live\n```",
    "Done.\n```bash\necho COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\necho done-live\n```",
]


def main() -> int:
    env = dict(os.environ)
    env["PYTHONPATH"] = str(HARNESS)
    env["BASHY_HINTS"] = "off"
    env.setdefault("BASHY_BIN", "")
    with run_server(REPLIES) as base_url:
        proc = subprocess.run(
            [sys.executable, "-m", "minisweagent_bounded.cli",
             "-t", "exercise the live transport", "-y",
             "--model-class", "openai", "--base-url", base_url, "-m", "fake-model",
             "--emit-envelope"],
            capture_output=True, text=True, env=env, cwd=str(HARNESS), timeout=30,
        )
    if proc.returncode != 0:
        print(f"live: FAIL rc={proc.returncode}\n{proc.stderr}", file=sys.stderr)
        return 1
    env_out = json.loads(proc.stdout)
    ok = (
        env_out["exit_status"] == "Submitted"
        and env_out["submission"] == "done-live\n"
        and env_out["model_name"] == "fake-model"
        and env_out["api_calls"] == 3
        and [a["command"] for a in env_out["actions"]] == [
            "echo recovered-live",
            "echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\necho done-live",
        ]
    )
    print(f"live: exit_status={env_out['exit_status']} api_calls={env_out['api_calls']} "
          f"submission={env_out['submission']!r} -> {'PASS' if ok else 'FAIL'}")
    return 0 if ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
