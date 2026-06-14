# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`bashy` is a pure-Go **Bash 5.3 drop-in** — a single static binary that runs
Bash scripts and interactive sessions with the same flags and semantics as
`bash` 5.3. It is the user-facing shell; the interpreter engine lives in the
[`qiangli/sh`](https://github.com/qiangli/sh) fork of `mvdan.cc/sh` (published
as the Go module `mvdan.cc/sh/v3`), which carries the unmerged Bash 5.3
interpreter patches.

This repo is **just the CLI + its compliance harness**: flag parsing, prompt
expansion, startup files, version vars, the interactive loops, and the bash
5.3 test-suite runner. The actual shell semantics (parameter expansion,
arrays, namerefs, `[[ ]]`, arithmetic, builtins, …) live in `mvdan.cc/sh/v3`'s
`interp`/`expand`/`syntax` packages. A feature that needs an interpreter
change is edited in `../sh`; this repo measures it via `make test-bash`.

### Source files (package `main`, repo root)

- `main.go` — entry point: flag parsing, runner setup, script/command/stdin
  dispatch, startup-file loading, bash-format parse-error remapping, static
  alias expansion.
- `interactive.go` — readline-backed interactive loop (delegates to
  `mvdan.cc/sh/v3/interactive`).
- `forced_interactive.go` — minimal readline emulation for `bash -i` with a
  non-TTY stdin (history, C-r/C-p, multi-line accumulation).
- `prompt.go` — Bash prompt escape expansion (`\u`, `\h`, `\w`, `\D{}`, …).
- `version.go` — `bashVersion` (a `var`, stampable via
  `-ldflags "-X main.bashVersion=..."`) and the `BASH`/`BASH_VERSION` env vars.
- `main_test.go` — CLI-level tests.

## Module wiring

`go.mod` requires `mvdan.cc/sh/v3` and resolves it as a flat sibling:

```
replace mvdan.cc/sh/v3 => ../sh
```

Inside the `dhnt/` umbrella, `../sh` is the `sh` submodule. In a standalone
clone, run `./scripts/bootstrap-siblings.sh` — it clones `github.com/qiangli/sh`
next to this repo as `../sh` at the SHA pinned in `.sibling-pins` (and leaves
an umbrella-mounted submodule alone). CI does the same before building. This
mirrors how `outpost` and `ycode` consume the fork — keep the sibling SHA
coordinated; the umbrella's `script/sync.sh` auto-bumps `.sibling-pins` when it
pulls a new `sh`, so standalone CI builds against the same SHA the monorepo
does.

## Build / test / lint

```sh
make build              # -> bin/bashy (also copies to bin/bash for the harness)
make test               # go test ./...
make test-bash          # drive bin/bash against bash's own 5.3 test suite
make test-bash-list     # list available fixtures
make dist               # cross-compile static binaries for all 6 platforms
make tidy               # go mod tidy + gofmt -s -w . + go vet ./...
```

Under finer-grained `go`:

```sh
go build ./...
go test ./...
go test -run TestMain ./...
```

### Local-env PATH gotcha (ycode shim)

If your `PATH` puts a `ycode` shim in front of `sh` (common on the dev
machine — `which sh` returns a `…/ycode-wrap/…/bin/sh`), Go tests that fork a
real shell can misbehave. Run the suite with a clean `PATH`:

```sh
PATH=/bin:/usr/bin:$(dirname $(which go)) go test ./...
```

## Workflow

At the start of every session, read `docs/TODO.md` and pick the first
unchecked item. After completing it, check it off, run `go test ./...` and
`make test-bash`, then commit. Repeat until the user says otherwise.

The goal is **PASS-count flips**: `make test-bash-list` prints per-fixture
PASS/FAIL/TIME/SKIP, and the headline three-tuple at the top of `docs/TODO.md`
(currently `72 passing, 4 failing, 11 skipped`) is the scoreboard. A change
that flips a fixture FAIL → PASS without regressing anything else is worth
shipping; cleanup that doesn't move the count isn't the priority. Most flips
require a change in `../sh` (interp/expand/syntax) plus, sometimes, the CLI
glue here.

**Scoreboard reliability.** `make test-bash` is unreliable when the ycode
shell wrapper shadows `sh` in `PATH` (see the gotcha above). To measure
reliably, drive `bin/bash` directly with the same environment the Makefile
sets up — export `BASH_TSTOUT`/`BASH_TSTRAW` to temp files,
`THIS_SH=$(pwd)/bin/bash`, a clean `PATH` (`$PWD:/usr/bin:/bin`), and mirror
the Makefile's per-fixture transforms: `BASH_TEST_FILTER_EXPECT` (strip
`expect `-prefixed lines before diff) and `BASH_TEST_CAT_V` (pipe through
`cat -v` for control-char fixtures like `printf`). `BASH_TEST_SKIP`
(`coproc jobs trap`) covers fixtures that hang on the goroutine-subshell /
no-kernel-job-control constraint. A diff that ignores these transforms will
false-positive; a checkout missing the `external/bash-5.3` fixture symlink
(gitignored) will false-pass because the fixtures aren't there to run.

### Bash 5.3 fixtures (gitignored symlink)

`external/bash-5.3` is a **gitignored symlink** into a Bash 5.3 source tree —
only its `tests/` dir is used. Set it up locally before running `make
test-bash`:

```sh
mkdir -p external
ln -s /path/to/bash-5.3 external/bash-5.3
```

`make test-bash-helpers` compiles the `recho`/`zecho` C helpers the suite
needs (the only place `cc` is invoked — for test fixtures, not for bashy
itself, which is pure Go).

### Doc index

`docs/` holds the planning + status corpus. Load-bearing entries:

- `TODO.md` — phase checklist + current PASS/FAIL/SKIP headline. Always read first.
- `report-bash53-test-status.md` — per-fixture status snapshot from the bash 5.3 suite.
- `handoff-bashy-2026-06.md` — most recent session-handoff notes (read when picking up cold).
- `bash-gap-analysis.md` — ungated bash semantics gap analysis behind the failing fixtures.
- `plan-bashy-drop-in.md` / `plan-cmd-bashy.md` / `plan-bash53-roadmap-agentic.md` — phase plans; each phase lands as a checkbox in `TODO.md`.
- `plan-dynvar.md`, `plan-error-format-pass.md`, `plan-punted-builtins.md` — scoped sub-plans for specific clusters of fixture failures.
- `json-output.md` — bashy's opt-in `set --json` / `declare --json` structured-output extensions.
- `bash.md`, `agentic-extensions.md` — background references, not active plans.

Per-fixture cluster analyses + blocker ledgers (snapshots — diff line-counts and PASS/FAIL claims in them are dated, re-measure before trusting):

- `ARITH-ANALYSIS.md`, `ARRAY-ANALYSIS.md`, `ASSOC-ANALYSIS.md`, `DBG-SUPPORT-ANALYSIS.md`, `NAMEREF-ANALYSIS.md`, `NEWEXP-ANALYSIS.md` — failure-cluster breakdowns for the named fixtures.
- `NEWEXP-RESIDUE-R2.md`, `ERRORS-ANALYSIS-R2.md` — round-2 residue analyses.
- `ERRORS-BLOCKERS.md`, `HEREDOC-BLOCKERS.md`, `HISTORY-BLOCKERS.md`, `QUOTEARRAY-BLOCKERS.md`, `VARENV-BLOCKERS.md` — per-fixture blocker ledgers.

Weave-round verification + retro reports (historical, not load-bearing):

- `QA-REPORT-R10.md`, `JUDGE-REPORT-R6.md`, `JUDGE-REPORT-R7.md`, `SPRINT-R10-RETRO-DRAFT.md`.

## Plans

Always save a copy of all implementation plans in `docs/`. Use a descriptive
filename (e.g. `docs/plan-feature-name.md`).

## Third-Party Libraries

- **Permissive licenses only**: MIT, BSD, Apache 2.0, or equivalent. No GPL/LGPL.
- **Pure Go only**: no CGo, no C dependencies (the `cc` invocation in
  `test-bash-helpers` builds Bash's own test helpers, not bashy).
