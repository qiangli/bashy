# `bashy dag` — markdown-defined dependency DAG

`bashy dag` runs targets defined as headings in a markdown file (`DAG.md`) as a
real dependency graph — an agent-first replacement for `make`. Each target is a
heading + optional metadata lines (`Requires:`/`Inputs:`/`Sources:`/`Generates:`/
`Host:`) + a fenced code block run through the in-process shell + coreutils
userland. `--list`/`--json` for discovery, `--explain` for a dry plan, content-
hashed up-to-date skip, topological execution.

Local bodies run **identically on Linux/macOS/Windows** (in-process shell +
coreutils — no PATH variance).

```bash
bashy dag --list                 # show targets (+ --json for machine output)
bashy dag build                  # run "build" and its dependencies
bashy dag pipeline.md ci         # run a target in a named file
```

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
bashy dag --mesh DAG.md build-on-node
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
