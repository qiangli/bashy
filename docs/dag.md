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
  and `~~~rs`/`~~~rust` are); `as py` is the alias the body calls through, and without an alias the
  fence's public functions are promoted into the body's namespace (`main()`).
- The fence's functions run in a persistent worker inside the project's own
  Python environment — the nearest `.venv` (or `.python-version`, an active
  `VIRTUAL_ENV`, then `python3` on PATH), discovered from the working
  directory. `BASHPP_PYTHON` overrides the executable. Nothing is installed:
  make the venv a `Requires:` dependency (a `sync` target).
- The fence is a declaration unit: only top-level `def`s; imports go inside
  the function body. Return values cross by value (str/int/float/bool/bytes/
  lists/maps); anything else is an opaque handle.
- A `~~~ts` fence works the same way for TypeScript: the nearest
  `package.json` + lockfile is the project (the manager — npm, pnpm, Bun — is
  read from `packageManager`/the lock and never invoked), the checker is the
  project-local `node_modules/typescript`, and the runtime is Node by default
  or Bun under `Env: BASHPP_TYPESCRIPT_RUNTIME=bun` (`BASHPP_NODE`/`BASHPP_BUN`
  name the executables). Ordinary ESM `import`s of the checkout's own source
  and packages are allowed; a relative import may name its `.ts` file
  explicitly, the spelling Node's native type stripping requires. Returned
  promises are awaited. One body may declare a `~~~py` fence AND a `~~~ts`
  fence and call both.
- A `~~~rust` fence exports top-level `pub fn` declarations. Bash++ invokes a
  native worker compiled by the nearest project-selected `rustc` (or
  `BASHPP_RUSTC`); `Cargo.toml`, `Cargo.lock`, and `rust-toolchain*` participate
  in environment identity, but Cargo is not run and dependencies are not
  installed implicitly. Parameters may be booleans, Rust integer/float
  primitives, `String`, `&str`, or `Vec<u8>`; results may use the owned forms,
  primitives, `Vec<u8>`, `()`, or `Result<T, E>` where `E: Display`. Return a
  `String`, not a borrowed `&str`. Function stdout/stderr and Rust errors cross
  the same Bash++ call boundary as other fences. The compiled artifact is
  embedded when Bash++ is lowered, so the resulting native program does not
  need `rustc` at run time. Rust code is native code with the task's host
  permissions; a fence is not a security sandbox. In a dag body the worker
  runs in the invoking cwd, so relative paths are the checkout's, and a
  `std::process::Command` on the repo's own built binary (`Requires: build`)
  is the launcher shape for a Cargo repo — `rs.launch()` in the examples.
  The fence is std-only, so it compiles with whatever `rustc` PATH resolves
  to; the repo's `cargo …` targets still need the toolchain the workspace
  pins (rustup's proxies pick it up; a distro `cargo` below the MSRV refuses
  the build, which is the repo's rule, not the fence's).
- Nesting rule: the dag parser closes a body only on a line equal to the
  **opening** marker, so a `~~~py … ~~~` block nests inside a ` ```bashpp `
  recipe. A recipe that itself opens with `~~~` cannot contain one.

Worked examples — two Python repos with a fenced-Python `smoke` target, two
TypeScript repos with a fenced-TypeScript one, one Python + TypeScript repo
whose `smoke` calls both from a single body, and three Rust repos (uv, Codex,
Bun) whose `smoke` reads the workspace's coordinates from a `~~~rs` fence and
whose `run` launches the freshly built CLI from one — live in
[`examples/dag/`](../examples/dag/) and are gated by `make smoke-dag-python`,
`make smoke-dag-typescript` and `make smoke-dag-rust`.

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
