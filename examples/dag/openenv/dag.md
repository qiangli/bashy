---
name: openenv
description: bashy dag front door for OpenEnv — sync, focused core unit tests, and an in-process echo-environment smoke, as one dependency graph
type: dag
default: test
vars:
  PYTEST_ARGS ?= -q
---

# OpenEnv — task graph

The repo's own commands (`uv`, `pytest`, the `justfile`'s recipes), wired as
a dependency graph so one front door replaces the ad-hoc `uv sync && pytest …`
incantations: `bashy dag --list` shows the targets, `bashy dag test` runs the
core unit suite after syncing, `bashy dag smoke` drives the checkout's echo
environment in process. Lives at the repo root of
[huggingface/OpenEnv](https://github.com/huggingface/OpenEnv) — formerly
`meta-pytorch/OpenEnv`; that URL redirects — (BSD-3-Clause); the pinned
example is measured at release tag `v0.8.0`
(`625f5e724f0672ebe4f98a5a29198325aedd1c36`, 2026-10-08). Needs `bashy`
(github.com/qiangli/bashy) and `uv` on PATH.

## Tasks

### sync
Create or refresh `.venv` — the repo's own install recipe (`just install`):
`uv sync --all-extras`, which also installs the `dev` group (pytest,
pytest-asyncio, ruff). OpenEnv does not commit a root `uv.lock` (each
environment under `envs/` keeps its own), so `uv` resolves from
`pyproject.toml` and keeps its lock local. `uv` fetches a managed CPython
when the host has none that satisfies `requires-python` (>= 3.10);
`--all-extras` includes the `harbor` extra only on Python >= 3.12 (the
upstream marker gate).
Sources: pyproject.toml
Generates: .venv/pyvenv.cfg
Effects: read, write, net
Timeout: 30m

```bash
uv sync --all-extras
```

### test
The focused unit suite: `tests/core`, the library's own core tests, run with
upstream's own selection — the `-m "not integration and not network and not
docker"` filter its `justfile` and CI both use — under `PYTHONPATH=src:envs`,
the dev PYTHONPATH its `justfile` exports. The whole `tests/` tree
additionally needs test-only dependencies CI installs by hand (numpy, nltk,
smolagents, python-chess, moonfish) and is not run from this front door.
Requires: sync
Env: PYTHONPATH=src:envs
Effects: read, write

```bash
uv run --no-sync pytest tests/core -m "not integration and not network and not docker" $PYTEST_ARGS
```

### smoke
Drive the checkout's echo environment IN PROCESS from a Bash++ body: a `~~~py`
fence declares a launcher, `py.echo()` imports `EchoEnvironment` from
`envs/echo_env` and the OpenEnv core (through the repo's own dev PYTHONPATH),
runs `reset()` then `step()` with a `CallToolAction`, and hands both
observations back to the shell, which asserts them — no server, no WebSocket,
no `python -c` quoting; the fence IS the launcher.
Requires: sync
Env: PYTHONPATH=src:envs
Effects: read

```bsh
~~~py as py
def echo() -> str:
    from openenv.core.env_server.mcp_types import CallToolAction
    from echo_env.server.echo_environment import EchoEnvironment

    env = EchoEnvironment()
    reset = env.reset()
    message = str(reset.metadata.get("message", ""))
    stepped = env.step(
        CallToolAction(
            tool_name="echo_message",
            arguments={"message": "hello from bashy dag"},
        )
    )
    result = stepped.result.data
    if not isinstance(result, str):
        result = str(result)
    return message + "|" + result
~~~
got := py.echo()
want="Echo environment ready!|hello from bashy dag"
[ "$got" = "$want" ] || { echo "smoke: py.echo() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: reset + echo_message -> $got"
```
