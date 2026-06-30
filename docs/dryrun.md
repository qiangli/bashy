# bashy `--dryrun` — preview commands and destructive file ops without running them

`--dryrun` is a **bashy-only** extension: run a script's control flow, expansions,
and builtins, but **print external commands instead of executing them** — "xtrace
without side effects." It is the agent-/CI-facing safety net: see exactly what a
script *would* invoke and what it *would* destroy, before it does anything.

It is absent from the pure `bash` drop-in, inert under `--posix`, and adds **zero
regression** to bash 5.3 conformance (86/86) — see [Design](#design).

## Usage

```sh
bashy --dryrun script.sh              # dry-run the whole script
bashy --dryrun -c '<commands>'

# Runtime toggle — dry-run only part of a script:
set -o dryrun                          # external commands now print + skip
rm -rf build                           # reported, not run
set +o dryrun                          # back to real execution
```

`set -o dryrun` / `set +o dryrun` flip dry-run mid-script. The flag and the
option share the one spelling **`dryrun`** (matching `dag --dryrun`).

## What it shows

Builtins, variable assignments, `cd`, and `${}` expansions still run (so the
trace is meaningful); every **external** command is reported and skipped.

### Human mode (default)

```
$ bashy --dryrun -c 'echo hi; rm -rf build; missing-tool'
hi
+ rm -rf build   # ⚠ DESTROYS 247 file(s), 1.2 GB
    e.g. build/a.o, build/cache/x, …
+ missing-tool   # MISSING on this system
```

- `+ argv` per external command (like `set -x`), with minimal shell-quoting.
- `# MISSING on this system` when the command would not resolve.
- `# ⚠ DESTROYS N file(s), <size>` for `rm` (and `# ⚠ TRUNCATES existing <size>`
  for a `>` clobber), with a sample of the affected paths.

### Agent mode (`BASHY_AGENTIC=1`)

A clean **JSON-lines manifest** on stdout (the script's own stdout is suppressed),
one event per distinct command plus destructive-op events — a dependency/security
preflight an agent can parse:

```json
{"kind":"command","command":"go","available":true,"resolved":"…/go/1.26.0/bin/go","args":["go","build"]}
{"kind":"command","command":"docker","available":false,"args":["docker","build","."]}
{"kind":"destroy","op":"rm","recursive":true,"paths":["build"],"files":247,"bytes":1288490188,"sample":["build/a.o",…]}
{"kind":"truncate","path":"config.yaml","bytes":4096}
```

- `command` — every distinct external command + whether it resolves on this
  system (coreutils in-process tool, or a binary on the runner's PATH). A
  `"available":false` is a missing dependency.
- `destroy` — what an `rm` would delete (count, total bytes, sample paths),
  computed by walking the real filesystem read-only.
- `truncate` — a `>`-style clobber of an existing file (size that would be lost).

## What it catches

| Op | How | Side effect during dry-run |
|---|---|---|
| any external command | ExecHandler intercepts argv | none (printed, skipped, returns 0) |
| `rm` / `rm -rf` | parse argv, walk the real FS read-only | none (reports files + bytes) |
| `> file` (truncate) | `interp.OpenHandler` records `O_TRUNC` on an existing file, returns a discard handle | none (the file is **not** truncated) |
| `< file`, `>> file` reads | pass through to the real open | safe reads only |

Verified: `rm -rf x; echo y > existing` under `--dryrun` reports both and leaves
every file untouched.

## Design

- **bashy-only, conformance-safe.** The `--dryrun` flag is registered in
  `internal/agentos` (imported only by `cmd/bashy`), so the pure `bash` drop-in
  never sees it. The `set -o dryrun` option is gated by a new
  `interp.EnableDryRunOption` RunnerOption that **only bashy passes** (and not
  under `--posix`); everywhere else — `bash`, `gosh`, `--posix` — `set -o dryrun`
  is rejected exactly like Bash. The option is kept out of `set -o` listings and
  `SHELLOPTS`. Bash 5.3 suite stays **86/86**.
- **Two seams, no engine rewrite.** A print-and-skip `interp.ExecHandler`
  (external commands) plus an `interp.OpenHandler` (redirection opens). Both no-op
  when `HandlerContext.DryRun()` is false, so normal runs are unaffected.
- Source: `internal/agentos/dryrun.go` (handlers, manifest, `rm`/`>` analysis);
  `internal/agentos/agentos.go` (`WireExec`); the gated option lives in the `sh`
  fork (`interp/api.go` `EnableDryRunOption`, `interp/handler.go`
  `HandlerContext.DryRun`).

## Limitations (and follow-ups)

- **Linear-path accurate, branch/loop approximate.** Skipped commands return 0,
  so `if grep …; then A; else B; fi` takes the `A` branch and `for x in $(ls)` does
  not iterate. The manifest reflects the executed (success-assumed) path, not all
  reachable commands. A **static all-branches audit** (walk the AST for every
  command) is the planned complete view for security use.
- **`rm`-only destruction** in v1; `mv`/`dd`/`truncate` join `analyzeDestroy`
  trivially (one `case` each).
- **No cumulative simulation** yet — a `rm` then a later `ls` does not see the
  deletion. A lazy copy-on-write **in-memory VFS twin** (Stage 2) would simulate
  cumulative effects + emit a final created/deleted/modified diff.
- Availability resolves against the runner's PATH at the moment the command would
  run (so a script-set `PATH=` is honored), but version-manager toolchains not on
  that PATH read as missing — activate them first (e.g. mise shims).
