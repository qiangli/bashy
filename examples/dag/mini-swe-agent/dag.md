---
name: mini-swe-agent
description: bashy dag front door for mini-SWE-agent — sync, test, lint, docs, and the CLI entry point, as one dependency graph
default: test
vars:
  PYTEST_ARGS ?= -q -n auto
---

# mini-SWE-agent — task graph

The repo's own commands (`uv`, `pytest`, `ruff`, `mkdocs`, `mini`), wired as
a dependency graph so one front door replaces the ad-hoc `uv sync && pytest …`
incantations: `bashy dag --list` shows the targets, `bashy dag test` runs the
suite after syncing. Lives at the repo root; needs `bashy`
(github.com/qiangli/bashy) and `uv` on PATH.

## Tasks

### sync
Create or refresh `.venv`: the `dev` dependency group plus the `dev` extra
(pytest-xdist, ruff, mkdocs). The repo does not commit `uv.lock` (it is
gitignored), so `uv` resolves from `pyproject.toml` and keeps its lock local.
`uv` fetches a managed CPython when the host has none that satisfies
`requires-python`.
Sources: pyproject.toml
Generates: .venv/pyvenv.cfg
Effects: read, write, net
Timeout: 30m

```bash
uv sync --group dev --extra dev
```

### test
Run the test suite the way CI does (`pytest -n auto`), minus the `slow`
marker. `tests/environments/extra` needs the `full` extra (swe-rex, modal,
contree) and is skipped here — see `test-full`.
Requires: sync
Effects: read, write

```bash
uv run --no-sync pytest -k "not slow" --ignore=tests/environments/extra $PYTEST_ARGS
```

### test-full
The whole suite including the extra environments. Syncs the `full` extra
first (heavier: swe-rex, modal, boto3); some of those tests need Docker or
credentials and skip themselves without them.
Requires: sync
Effects: read, write, net

```bash
uv sync --group dev --extra full
uv run --no-sync pytest -k "not slow" $PYTEST_ARGS
```

### lint
`ruff check` over sources and tests (the pre-commit hook's linter).
Requires: sync
Effects: read

```bash
uv run --no-sync ruff check src tests
```

### docs
Build the MkDocs site (`--strict`: a broken link fails the build).
Requires: sync
Generates: site/index.html
Effects: read, write

```bash
uv run --no-sync mkdocs build --strict
```

### run
Prove the `[project.scripts]` entry point resolves in the synced venv.
Requires: sync
Effects: read

```bash
uv run --no-sync mini --help
```

### smoke
Call the package directly from a Bash++ body: a `~~~py` fence declares a
launcher, `py.main()` resolves the default agent class through the project's
venv and hands its name back to the shell. No wrapper script, no subprocess
plumbing — the fence IS the launcher. (`PYTHONPATH=src` names the source tree
as the import root so the smoke does not depend on the editable install.)
Requires: sync
Env: PYTHONPATH=src
Effects: read

```bashpp
~~~py as py
def main() -> str:
    from minisweagent.agents import get_agent_class
    return get_agent_class("default").__name__
~~~
name := py.main()
[ "$name" = DefaultAgent ] || { echo "smoke: get_agent_class('default') -> '$name'" >&2; exit 1; }
echo "smoke: get_agent_class('default') -> $name"
```
