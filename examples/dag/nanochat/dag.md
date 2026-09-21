---
name: nanochat
description: bashy dag front door for nanochat — sync, test, smoke, and the training/chat entry points, as one dependency graph
default: test
vars:
  PYTEST_ARGS ?= -q
---

# nanochat — task graph

The repo's own commands (`uv`, `pytest`, `python -m scripts.*`), wired as a
dependency graph so one front door replaces the ad-hoc `uv sync && pytest …`
incantations: `bashy dag --list` shows the targets, `bashy dag test` runs the
CPU-safe suite after syncing. Lives at the repo root; needs `bashy`
(github.com/qiangli/bashy) and `uv` on PATH.

## Tasks

### sync
Create or refresh `.venv` from `uv.lock` (CPU torch, dev group) — the same
`uv sync --extra cpu --group dev` the README documents, `--frozen` so the
committed lock is honoured as-is and never rewritten by a newer `uv`. `uv`
honours `.python-version` and fetches a managed CPython when the host has none.
Sources: pyproject.toml uv.lock .python-version
Generates: .venv/pyvenv.cfg
Effects: read, write, net
Timeout: 30m

```bash
uv sync --frozen --extra cpu --group dev
```

### test
Run the CPU-safe test suite: `-m "not slow"` as the repo's pytest config
documents (`PYTEST_ARGS=…` on the command line adds to it); CUDA-only tests
skip themselves. `python -m pytest` (not bare
`pytest`) so the repo root is importable — nanochat is a uv-native project
with no build backend, so nothing is installed into the venv. On macOS
`test_memory_limit` is deselected: the RLIMIT_AS cap it asserts is not
enforced there (a platform limit, not a nanochat defect).
Requires: sync
Effects: read, write

```bash
deselect=""
[ "$(uname -s)" = Darwin ] && deselect="--deselect tests/test_execution.py::test_memory_limit"
uv run --no-sync python -m pytest -m "not slow" $PYTEST_ARGS $deselect
```

### smoke
Call the package directly from a Bash++ body: a `~~~py` fence declares a
launcher, `py.main()` runs `nanochat.execution.execute_code` in the project's
venv and hands the result back to the shell. No wrapper script, no
subprocess plumbing — the fence IS the launcher. `PYTHONPATH=.` names the
checkout as the import root, since nothing is installed.
Requires: sync
Env: PYTHONPATH=.
Effects: read

```bsh
~~~py as py
def main() -> str:
    from nanochat.execution import execute_code
    result = execute_code("print(6 * 7)", timeout=5)
    if not result.success:
        return "error: " + (result.error or result.stderr or "unknown")
    return result.stdout.strip()
~~~
answer := py.main()
[ "$answer" = 42 ] || { echo "smoke: execute_code returned '$answer'" >&2; exit 1; }
echo "smoke: execute_code -> $answer"
```

### runcpu
The README's CPU/MPS bring-up (`runs/runcpu.sh`): downloads data, trains a
tiny model, evaluates. Long-running and network-bound — documented here as a
target, not part of any quick gate.
Requires: sync
Effects: read, write, net
Timeout: 6h

```bash
bash runs/runcpu.sh
```

### chat-cli
Talk to a trained model (`python -m scripts.chat_cli`). Interactive; needs a
checkpoint from `runcpu` or `runs/speedrun.sh`.
Requires: sync
Effects: read

```bash
uv run --no-sync python -m scripts.chat_cli
```
