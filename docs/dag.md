# `bashy dag` — markdown-defined dependency DAG

`bashy dag` runs targets defined as headings in a markdown file (`dag.md` at the
repo root, or any file you pass) as a
real dependency graph — an agent-first replacement for `make`. Each target is a
heading + optional metadata lines (`Requires:`/`Inputs:`/`Sources:`/`Generates:`/
`Host:`) + a fenced code block run through the in-process shell + coreutils
userland. `--list`/`--json` for discovery, `--explain` for a dry plan, content-
hashed up-to-date skip, topological execution.

Local bodies run **identically on Linux/macOS/Windows** (in-process shell +
coreutils — no PATH variance).

When a target body needs to invoke bashy again, prefer `"$BASHY" ...` over a
bare `bashy ...`. Mirroring GNU Bash's `BASH`/`BASH_ARGV0` split, the runner
injects `BASHY` and `BASHY_EXE` as the resolved executable path for the current
`bashy dag` process, and `BASHY_ARGV0` as the raw argv0 string. Recursive
DAG/tool calls should use `"$BASHY"` so they stay on the same binary version
instead of whichever `bashy` happens to be first on `PATH`.

```bash
bashy dag --list                 # show targets (+ --json for machine output)
bashy dag build                  # run "build" and its dependencies
bashy dag pipeline.md ci         # run a target in a named file
```

## Working directory

Bodies run in the **invoking working directory**, exactly as `make` recipes
do: `-f FILE` (or a positional file) only selects the task file, so a graph
kept elsewhere — a checked-in example, a shared pipeline — drives the checkout
you are standing in. `Sources:`/`Generates:`/`Inputs:`/`Artifacts:` and
`Ensure:` resolve against that same directory; `include:` and `chunks.json`
stay file-relative because they describe the file, not the run.

There is deliberately **no `-C DIR` flag** — on `dag` or on any other verb.
`awd DIR -- CMD` ("run one command over there, come back") is the one directory
mechanism, and it is a front-door verb as well as a shell builtin:

```bash
bashy awd ~/src/nanochat -- bashy dag -f ~/pipelines/nanochat.dag.md test
```

The one exception is a positional **directory**: `bashy dag .bashy/deploy
target` names both the file (that folder's `dag.md`) and the place to run it.
`--explain` prints the effective directory.

## Bash++ bodies and foreign fences

A body tagged ` ```bashpp ` (alias ` ```bash++ `) runs as Bash++ instead of
Classic Bash. Untagged and ` ```bash ` bodies are unchanged — Bash++ is opted
into per target, so no existing task file is reinterpreted.

A Bash++ body may declare a **source fence** in another language and call its
functions directly. The launcher shape:

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

- `~~~py` and `~~~python` are the same language (as `~~~ts`/`~~~typescript`
  are); `as py` is the alias the body calls through, and without an alias the
  fence's public functions are promoted into the body's namespace (`main()`).
- The fence's functions run in a persistent worker inside the project's own
  Python environment — the nearest `.venv` (or `.python-version`, an active
  `VIRTUAL_ENV`, then `python3` on PATH), discovered from the working
  directory. `BASHPP_PYTHON` overrides the executable. Nothing is installed:
  make the venv a `Requires:` dependency (a `sync` target).
- The fence is a declaration unit: only top-level `def`s; imports go inside
  the function body. Return values cross by value (str/int/float/bool/bytes/
  lists/maps); anything else is an opaque handle.
- Nesting rule: the dag parser closes a body only on a line equal to the
  **opening** marker, so a `~~~py … ~~~` block nests inside a ` ```bashpp `
  recipe. A recipe that itself opens with `~~~` cannot contain one.

Worked examples — two real Python repos driven by one `dag.md` each, with a
fenced-Python `smoke` target — live in [`examples/dag/`](../examples/dag/)
and are gated by `make smoke-dag-python`.

## Cross-machine dispatch — `--mesh`

A target carrying a `Host:` line is dispatched to **another machine** under
`--mesh`:

````markdown
## build-on-node
Build the artifact on a remote node.
Host: some-node

```bash
# this body runs ON some-node; it fetches its own code/data
git clone … && cmake -B build … && cmake --build build
```
````

```bash
bashy dag --mesh dag.md build-on-node
```

It is **control-plane only**: the body is fed to the remote over an exec
transport (default `ssh <host>`, override with `--remote` or the
`DAG_REMOTE_EXEC` env var) — nothing is shipped over the channel, so the body
fetches its own code/data. Any `ssh`-compatible transport works, including an
agent ssh-proxy stanza in `~/.ssh/config` (`ProxyCommand …`).

## Windows hosts (mesh targets)

A `--mesh` body runs in the **remote host's** shell, so the "identical on every
platform" guarantee — which covers *local* bodies — does not extend to it. When
the remote is a Windows host reached through an agent shell with a **minimal
PATH** (e.g. a remote agent ssh session, whose PATH is essentially just the agent's
own directory — `C:\Windows\System32` is **not** on it), a bare
`cmd`/`curl`/`tar`/`nvidia-smi` reports `executable file not found in $PATH`.

Spawning a Windows `.exe` works fine — you just have to name it where the
minimal PATH can't help:

- **`"$COMSPEC" /c "<windows command>"`** — `cmd.exe` (always at `$COMSPEC`)
  runs the command with the **full** Windows PATH, so `curl`, `tar`,
  `nvidia-smi`, … resolve normally.
- or a **native backslash absolute path** (`C:\path\to\tool.exe`). Forward-slash
  (`/c/...`) and msys-style paths do **not** resolve in this shell.

To stage files, prefer **scp** (e.g. `scp <file> <host>:<dest>`, optionally
through an `ssh-proxy` stanza) over fetching inside the body — copy the
binary/inputs over, then launch via `"$COMSPEC" /c` or a full path. Pure-Go
coreutils builtins (`cat`, `ls`, `grep`, …) work regardless — they resolve
in-process, not via PATH.
