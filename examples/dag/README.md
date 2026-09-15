# `bashy dag` as a project's front door

Five real repositories, each driven by ONE `dag.md` at its root instead of a
`Makefile`, a `justfile`, an `npm run`/`pnpm`/`uv run` alias list, or a README
full of incantations:

| example | repo | stack | targets |
|---|---|---|---|
| [`mini-swe-agent/dag.md`](mini-swe-agent/dag.md) | [SWE-agent/mini-swe-agent](https://github.com/SWE-agent/mini-swe-agent) | Python (uv) | `sync` · `test` · `test-full` · `lint` · `docs` · `run` · `smoke` |
| [`nanochat/dag.md`](nanochat/dag.md) | [karpathy/nanochat](https://github.com/karpathy/nanochat) | Python (uv) | `sync` · `test` · `smoke` · `runcpu` · `chat-cli` |
| [`opencode/dag.md`](opencode/dag.md) | [anomalyco/opencode](https://github.com/anomalyco/opencode) | TypeScript (Bun workspace) | `install` · `typecheck` · `lint` · `test` · `test-config` · `run` · `smoke` |
| [`openclaw/dag.md`](openclaw/dag.md) | [openclaw/openclaw](https://github.com/openclaw/openclaw) | TypeScript (pnpm workspace, Node) | `install` · `typecheck` · `format-check` · `lint` · `build` · `test` · `test-unit-fast` · `test-file` · `run` · `smoke` |
| [`hermes-agent/dag.md`](hermes-agent/dag.md) | [NousResearch/Hermes-Agent](https://github.com/NousResearch/Hermes-Agent) | Python (uv) + TypeScript (npm workspace) | `sync` · `test` · `lint` · `run` · `install-tui` · `build-ink` · `typecheck-tui` · `test-tui` · `smoke` |

Each target wraps the repo's OWN command (`uv sync`, `pytest`, `ruff`,
`mkdocs`, `bun test`, `tsgo`, `oxlint`, `pnpm tsgo:core`, `vitest`, `npm run
typecheck`, …) and declares what it depends on (`Requires:`), what it reads
and produces (`Sources:`/`Generates:` — an unchanged `install` is skipped by
content hash, not by mtime), and what it is allowed to do (`Effects:`).
`bashy dag --list` is the help; `bashy dag test` does the right thing in
order; `-j` parallelises; `--json` gives an agent one envelope.

## The `smoke` target: a fence as the launcher

Every file carries one target whose body is Bash++ (` ```bashpp `) and
declares its launcher IN the project's own language. Python:

````markdown
### smoke
Requires: sync
Env: PYTHONPATH=src

```bashpp
~~~py as py
def main() -> str:
    from minisweagent.agents import get_agent_class
    return get_agent_class("default").__name__
~~~
name := py.main()
[ "$name" = DefaultAgent ]
```
````

TypeScript — the same shape, importing the checkout's own source:

````markdown
### smoke
Requires: install
Env: BASHPP_TYPESCRIPT_RUNTIME=bun

```bashpp
~~~ts as ts
import { fileInDirectory } from "./packages/opencode/src/config/paths"
export function launch(): string {
  return fileInDirectory(".opencode", "opencode").join("|")
}
~~~
got := ts.launch()
[ "$got" = ".opencode/opencode.json|.opencode/opencode.jsonc" ]
```
````

`~~~py … ~~~` / `~~~ts … ~~~` are Bash++ source fences: the functions they
define become callable from the shell body (`py.main()`, `ts.launch()`), run
by a persistent worker in the project's own environment, discovered from the
working directory — no `activate`, no wrapper script, no `python -c` / `bun
-e` quoting. `~~~py`/`~~~python` and `~~~ts`/`~~~typescript` are the same
languages; `as py` / `as ts` is the alias the body calls through. The
`sync`/`install` dependency guarantees the environment exists first.

For Python the environment is the nearest `.venv`. For TypeScript it is the
nearest `package.json` + lockfile: the manager is read from
`packageManager`/the lock (npm, pnpm, Bun — never invoked), the checker is
the project-local `node_modules/typescript`, and the runtime is Node by
default — its native type stripping runs `.ts` sources as-is — or Bun when the
target says `Env: BASHPP_TYPESCRIPT_RUNTIME=bun`. Pick the runtime the repo's
own source assumes: OpenCode uses extensionless internal imports (Bun-only);
OpenClaw imports `.ts` neighbours by their `.js` output name (NodeNext — its
own scripts go through `tsx`, Bun resolves it natively); Hermes' TUI package
exports `.ts` files directly, which Node loads unaided. A relative import in
the fence may name its `.ts` file explicitly (`./src/x.ts`) — the spelling
Node requires — regardless of the project's `tsconfig.json`.

The Hermes example puts BOTH fences in one body: `py.version()` through the
venv and `ts.compact()` through the npm workspace, one shell body, two
languages, no glue.

## Running them against a checkout

The files are written to live at each repo's root (copy one there and run
`bashy dag`). To drive a checkout WITHOUT copying — the way the gates do —
use `awd`, bashy's "run one command over there":

```bash
bashy awd ~/src/nanochat -- bashy dag -f "$PWD/nanochat/dag.md" sync test smoke
bashy awd ~/src/opencode -- bashy dag -f "$PWD/opencode/dag.md" install typecheck smoke
```

`bashy dag` runs bodies in the invoking working directory, like `make`; `-f`
only picks the file. That is why there is no `-C DIR` flag on `dag` (or on any
other verb): `awd DIR -- CMD` is the one directory mechanism, and it works for
every command.

Two installed-product gates run the graphs this way against unchanged
checkouts and assert the JSON envelope, the fence results and a
byte-identical `git status` before and after:

- `make smoke-dag-python` (`scripts/dag-python-examples-smoke.sh`) —
  `MINISWEAGENT_ROOT`, `NANOCHAT_ROOT`.
- `make smoke-dag-typescript` (`scripts/dag-typescript-examples-smoke.sh`)
  — `OPENCODE_ROOT`, `OPENCLAW_ROOT`, `HERMESAGENT_ROOT`; needs `bun`
  (`BASHPP_BUN` if off PATH) and `pnpm` (or `corepack`). The heavy
  repo-wide lanes (`lint`, `test`, `test-unit-fast`) are documented targets,
  not gate targets: they report each checkout's own state, which is not
  bashy's to assert.
