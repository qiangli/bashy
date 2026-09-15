---
name: hermes-agent
description: bashy dag front door for Hermes Agent — the Python core (uv, pytest, ruff) and the TypeScript TUI workspace (npm, tsc, vitest), as one dependency graph
default: test
vars:
  TEST_PATHS ?= tests
  PYTEST_ARGS ?= -q
---

# Hermes Agent — task graph

The repo's own commands (`uv sync`, `scripts/run_tests.sh`, `ruff`,
`run_agent.py`, `npm ci --workspace ui-tui`, `tsc`, `vitest`), wired as a
dependency graph so one front door replaces the ad-hoc incantations: `bashy
dag --list` shows the targets, `bashy dag test` syncs the venv, then runs the
canonical test runner. Lives at the repo root; needs `bashy`
(github.com/qiangli/bashy), `uv`, `node` (the `engines` range) and `npm` on
PATH. Two languages, one graph: the Python core is a uv project
(`pyproject.toml` + committed `uv.lock`), the TUI is an npm workspace
(`package.json` + committed `package-lock.json`, `ui-tui/`).

## Tasks

### sync
Create or refresh `.venv` from the committed `uv.lock` with the `dev` extra
(pytest, ruff, ty) — `--frozen` so a newer `uv` never rewrites the lock.
`uv` honours `.python-version` and fetches a managed CPython when the host
has none. Note: running the `hermes` console script itself (even
`--version`) triggers Hermes' CVE-driven runtime repair, which rebuilds
`.venv` WITHOUT the dev extra — `run` below goes through `run_agent.py`
for that reason, and `sync` puts a repaired venv back.
Sources: pyproject.toml uv.lock .python-version
Generates: .venv/pyvenv.cfg
Effects: read, write, net
Timeout: 30m

```bash
uv sync --frozen --extra dev
```

### test
The canonical runner (`scripts/run_tests.sh`: CI-parity env, per-file
subprocess isolation, never bare `pytest`, as AGENTS.md insists). The whole
`tests/` tree by default; `TEST_PATHS=tests/skills` on the command line
narrows it to one directory (the runner is file-granular), `PYTEST_ARGS=…`
passes flags through.
Requires: sync
Effects: read, write

```bash
scripts/run_tests.sh $TEST_PATHS $PYTEST_ARGS
```

### lint
`ruff check .` — the blocking half of the repo's lint workflow.
Requires: sync
Effects: read

```bash
uv run --no-sync ruff check .
```

### run
Prove the agent entry point resolves in the synced venv: `run_agent.py
--help` (the `hermes-agent` console script's module), with `HERMES_HOME`
pointed at a scratch directory so no profile is created under `$HOME`.
Requires: sync
Effects: read, write

```bash
HERMES_HOME=${TMPDIR:-/tmp}/hermes-dag-home uv run --no-sync python run_agent.py --help >/dev/null
echo "run: run_agent.py --help ok"
```

### install-tui
Install the `ui-tui` npm workspace (React/Ink TUI) from the committed
`package-lock.json` — `npm ci`, never `npm install`, because a newer `npm`
rewrites the lock on install. Links `@hermes/shared` and `@hermes/ink` into
the root `node_modules` and hoists the workspace's TypeScript there.
Sources: package.json package-lock.json ui-tui/package.json
Generates: node_modules/typescript/package.json
Effects: read, write, net
Timeout: 30m

```bash
npm ci --workspace ui-tui
```

### build-ink
Build the vendored Ink fork (`@hermes/ink`) — the TUI tests import its
`dist/`, so the workspace's own `check` script runs this first.
Requires: install-tui
Generates: ui-tui/packages/hermes-ink/dist/entry-exports.js
Effects: read, write

```bash
npm run build:ink --workspace ui-tui
```

### typecheck-tui
`tsc --noEmit` over the TUI (the workspace's `typecheck` script).
Requires: install-tui
Effects: read

```bash
npm run typecheck --workspace ui-tui
```

### test-tui
`vitest run` over the TUI (the workspace's `test` script). One suite reads
the terminal program to pick a default palette, so run it from a plain
shell (or with `TERM_PROGRAM` unset) for a clean-terminal baseline.
Requires: build-ink
Effects: read, write

```bash
npm test --workspace ui-tui
```

### smoke
Call both halves of the repo directly from one Bash++ body: a `~~~py`
fence declares a Python launcher (`py.version()` reads the installed
`hermes-agent` distribution through the project's venv) and a `~~~ts` fence
declares a TypeScript launcher (`ts.compact()` imports `compactNumber` from
`@hermes/shared/format`, the workspace package the TUI itself imports,
checked by the workspace's own TypeScript compiler and run on Node — the
default runtime — with its native type stripping). No wrapper script, no
`python -c`/`node -e` quoting — the fences ARE the launchers.
Requires: sync install-tui
Effects: read

```bashpp
~~~py as py
def version() -> str:
    from importlib.metadata import version
    return version("hermes-agent")
~~~
~~~ts as ts
import { compactNumber } from "@hermes/shared/format"
export function compact(n: number): string {
  return compactNumber(n)
}
~~~
pyv := py.version()
tsv := ts.compact(1500000)
case "$pyv" in *.*.*) ;; *) echo "smoke: hermes-agent version -> '$pyv'" >&2; exit 1 ;; esac
[ "$tsv" = 1.5M ] || { echo "smoke: compactNumber(1500000) -> '$tsv'" >&2; exit 1; }
echo "smoke: hermes-agent $pyv; compactNumber(1500000) -> $tsv"
```
