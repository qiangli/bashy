# `bashy dag` as a project's front door

Two real Python repositories, each driven by ONE `dag.md` at its root instead
of a `Makefile`, a `justfile`, an `npm run`/`uv run` alias list, or a README
full of incantations:

| example | repo | targets |
|---|---|---|
| [`mini-swe-agent/dag.md`](mini-swe-agent/dag.md) | [SWE-agent/mini-swe-agent](https://github.com/SWE-agent/mini-swe-agent) | `sync` · `test` · `test-full` · `lint` · `docs` · `run` · `smoke` |
| [`nanochat/dag.md`](nanochat/dag.md) | [karpathy/nanochat](https://github.com/karpathy/nanochat) | `sync` · `test` · `smoke` · `runcpu` · `chat-cli` |

Each target wraps the repo's OWN command (`uv sync`, `pytest`, `ruff`,
`mkdocs`, `python -m scripts.chat_cli`) and declares what it depends on
(`Requires:`), what it reads and produces (`Sources:`/`Generates:` — an
unchanged `sync` is skipped by content hash, not by mtime), and what it is
allowed to do (`Effects:`). `bashy dag --list` is the help; `bashy dag test`
does the right thing in order; `-j` parallelises; `--json` gives an agent one
envelope.

## The `smoke` target: a Python fence as the launcher

Both files carry one target whose body is Bash++ (` ```bashpp `) and declares
its launcher IN Python:

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

`~~~py … ~~~` is a Bash++ source fence: the functions it defines become
callable from the shell body (`py.main()`), run by a persistent worker in the
project's own virtualenv (the nearest `.venv`, discovered from the working
directory — no `activate`, no wrapper script, no `python -c` quoting). `~~~py`
and `~~~python` are the same language; `as py` is the alias the body calls
through. The `sync` dependency guarantees the venv exists first.

## Running them against a checkout

The files are written to live at each repo's root (copy one there and run
`bashy dag`). To drive a checkout WITHOUT copying — the way the gate does —
use `awd`, bashy's "run one command over there":

```bash
bashy awd ~/src/nanochat -- bashy dag -f "$PWD/nanochat/dag.md" sync test smoke
```

`bashy dag` runs bodies in the invoking working directory, like `make`; `-f`
only picks the file. That is why there is no `-C DIR` flag on `dag` (or on any
other verb): `awd DIR -- CMD` is the one directory mechanism, and it works for
every command.

The installed-product gate `make smoke-dag-python` (`scripts/dag-python-
examples-smoke.sh`) runs both graphs this way against unchanged checkouts —
`MINISWEAGENT_ROOT` and `NANOCHAT_ROOT` name them — and asserts the JSON
envelope, the fence results (`DefaultAgent`, `42`) and a byte-identical
`git status` before and after.
