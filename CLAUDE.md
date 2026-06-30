# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

This repo builds **two independent binaries** that share a common shell core
(`internal/cli`) but are **separate compilations** — each has its own `main`
package under `cmd/`, so their import graphs are disjoint:

- **`bash`** (`cmd/bash`) — a pure-Go **Bash 5.3 drop-in**: runs Bash scripts
  and interactive sessions with the same flags and semantics as `bash` 5.3,
  resolving external commands through `PATH` exactly as bash does. Its import
  graph **never includes coreutils** or any AgentOS surface, so it stays lean
  (~8 MB vs. bashy's ~40 MB). **The compliance harness drives `bin/bash`, so
  the conformance work measures this pure drop-in.**
- **`bashy`** (`cmd/bashy`) — the **AgentOS system shell**: the same shell core
  plus the coreutils `shell.Handler()` ExecHandler (pure-Go userland
  cat/ls/grep/… and the `yc` code-intel verbs, in-process across
  Linux/macOS/Windows) and the front-door subcommands (`bashy weave …`,
  `bashy podman …`). It is the self-contained bootstrapper for a whole
  unix-like userland (bash + coreutils + pkg + external tools).

The AgentOS surface is injected, not branched at runtime: `internal/cli`
exposes two no-op hook vars (`AgentOSDispatch`, `AgentOSWireExec`); `cmd/bashy`
sets them to `internal/agentos.{Dispatch,WireExec}` in its `init()`, while
`cmd/bash` leaves the defaults. Because the coreutils import lives only in
`internal/agentos` (imported only by `cmd/bashy`), the `bash` binary cannot
pull it in. `make build` produces both `bin/bash` and `bin/bashy`. (Historical
note: this used to be one binary split by argv[0] via `isAgentOSShell()`; it is
now a structural cmd/ split — see docs/agentos-substrate-extraction-plan.md.)

The interpreter engine lives in the
[`qiangli/sh`](https://github.com/qiangli/sh) fork of `mvdan.cc/sh` (published
as the Go module `mvdan.cc/sh/v3`), which carries the unmerged Bash 5.3
interpreter patches.

This repo is **just the CLI + its compliance harness**: flag parsing, prompt
expansion, startup files, version vars, the interactive loops, and the bash
5.3 test-suite runner. The actual shell semantics (parameter expansion,
arrays, namerefs, `[[ ]]`, arithmetic, builtins, …) live in `mvdan.cc/sh/v3`'s
`interp`/`expand`/`syntax` packages. A feature that needs an interpreter
change is edited in `../sh`; this repo measures it via `make test-bash`.

### Source layout

- `cmd/bash/main.go` — pure drop-in entry point: `cli.Main()`, no AgentOS imports.
- `cmd/bashy/main.go` — AgentOS entry point: wires `internal/agentos` hooks into
  `internal/cli`, then `cli.Main()`.
- `internal/cli/` — the shared shell core (`package cli`):
  - `main.go` — `Main()`: flag parsing, runner setup, script/command/stdin
    dispatch, startup-file loading, bash-format parse-error remapping, static
    alias expansion; defines the `AgentOSDispatch`/`AgentOSWireExec` hook vars.
  - `interactive.go` — readline-backed interactive loop (delegates to
    `mvdan.cc/sh/v3/interactive`).
  - `forced_interactive.go` — minimal readline emulation for `bash -i` with a
    non-TTY stdin (history, C-r/C-p, multi-line accumulation).
  - `prompt.go` — Bash prompt escape expansion (`\u`, `\h`, `\w`, `\D{}`, …)
    plus posix parameter/`!!` prompt expansion (uses `Runner.LiveVar`).
  - `version.go` — `bashVersion` (a `var`, stampable via
    `-ldflags "-X github.com/qiangli/bashy/internal/cli.bashVersion=..."`).
  - `main_test.go` — CLI-level tests.
- `internal/agentos/agentos.go` — the AgentOS wiring (imports coreutils):
  `WireExec()` (coreutils ExecHandler) and `Dispatch()` (front-door subcommands
  `bashy weave …` via `coreutils/pkg/weave`; `bashy podman …` via
  `coreutils/external/podman/engine` — the **embedded, isolated** in-process
  podman engine, `CONTAINER_HOST` pinned to a private `bashy` machine, never a
  shared host one; `bashy ollama …` via `coreutils/external/ollama`'s
  `NewManagedOllamaCmd` — isolated daemon, own port/models; plus `bashy
  act-runner`, `loom`, `zot`, `seaweedfs`, `kopia`). Imported only by `cmd/bashy`,
  so the lean `bash` binary never links any of it.
  - `internal/agentos/advisor*.go` — the **space-time advisor**: a non-intrusive
    post-exec `ExecHandler` middleware that, only when a command fails, appends one
    advisory hint explaining a space-determined failure (wrong cwd, host gone
    remote, OOM, full/read-only disk) so an agent stops the doomed retry loop. Has
    its own memory (per-session doomed-loop counter + a persisted host-success
    ledger keyed by a network fingerprint). Agent-mode/`BASHY_ADVISOR` gated, off
    in `--posix`, never linked into `cmd/bash`. Self-contained — depends on no
    other feature. See `docs/space-time-advisor.md`.
  - **Bare-name verb shims** (`Preamble()`): front-door verbs are exposed without
    the `bashy ` prefix via overridable shell functions (`weave(){ command bashy
    weave "$@"; }`, …). Shadowing policy: native verbs + identical drop-in
    passthroughs (gh/act/rclone/podman/ollama/loom/zot/seaweedfs/kopia/mirror)
    always shimmed; version-sensitive provisioners (go/cmake/clang) only in agent
    mode; `time` (keyword) and jobs/fg/bg/kill (builtins) never. Override with
    `unset -f <name>`; reach a specific binary by absolute path.
  - **Embed tags:** the `Makefile` adds `-tags embed_podman/embed_vfkit/
    embed_gvproxy` to the `cmd/bashy` build for whichever
    `../coreutils/external/podman/engine/*_embed/*.gz` blobs exist (built by
    `coreutils/scripts/embed-*.sh`). With the blobs, `bashy podman` is fully
    self-contained (no host podman); without them it falls back to a PATH podman.
    `cmd/bash` never gets these tags. Embedding the engine makes `bin/bashy` large
    (~259 MB with blobs); `bin/bash` stays ~5.7 MB.
  - **Core vs ext / build profiles:** the default `cmd/bashy` is the **lean
    worker** — shell + coreutils userland + git + dag + `bashy go`
    (self-provisioning Go toolchain via `coreutils/external/gotoolchain` on
    binmgr's tree-mode `Ensure`) + weave/secrets/jobs/mirror + the binmgr-managed
    externals (loom/zot/seaweedfs/kopia/rclone — download-on-demand, not compiled
    in). It is pure-Go and **cross-compiles to every platform with
    `CGO_ENABLED=0`** (~121 MB unix, ~47 MB Windows) — this is what GoReleaser
    ships. Two opt-in, unix-only, heavier **host** layers, both default-EXCLUDED
    so the worker stays lean and portable:
    - `-tags bashy_engines` (`engines_{full,stub}.go`) — the container/LLM engines
      `bashy podman`/`ollama` (cgo + btrfs/MLX). Always excluded on Windows.
    - `-tags bashy_obs` (`obs_{full,stub}.go`) — the observability stack
      `bashy otel` (OpenTelemetry Collector + VictoriaMetrics/Logs + Jaeger +
      Perses + k8s/aws, **193 MB**).

    `make build` = lean; `make build-host` (= `BASHY_ENGINES=1 BASHY_OBS=1`,
    pulling in the embed blobs too) = full unix host. Rule of thumb: a worker
    essential that's pure-Go + cross-platform is **core** (compiled in); a heavy
    or cgo host service is **ext** (build-tag, or binmgr download-on-demand).

## Module wiring

`go.mod` requires two flat-sibling deps, resolved by `replace`:

```
replace mvdan.cc/sh/v3              => ../sh
replace github.com/qiangli/coreutils => ../coreutils
```

`../sh` is the interpreter engine; `../coreutils` is the AgentOS hub that
supplies the pure-Go userland + `yc` verbs the `bashy` binary injects (only
`agentos.go` imports it). In a parent monorepo both are submodules. In
a standalone clone, run `./scripts/bootstrap-siblings.sh` — it clones
`github.com/qiangli/{sh,coreutils}` next to this repo at the SHAs pinned in
`.sibling-pins` (and leaves any submodule mounts alone). CI does the
same before building. coreutils itself replaces `../sh`, which resolves to the
same flat sibling. Keep the sibling SHAs coordinated; a parent monorepo's
sync tooling auto-bumps `.sibling-pins`.

## Build / test / lint

```sh
make build              # -> bin/bash (pure drop-in, cmd/bash) + bin/bashy (AgentOS, cmd/bashy) — two independent binaries
make build-host         # full unix host build (= BASHY_ENGINES=1 BASHY_OBS=1 + embed blobs)
make test               # go test ./...
make test-bash          # drive bin/bash against bash's own 5.3 test suite (serial)
make test-bash-parallel # same suite fanned out across cores — the canonical 86/86 gate
make test-bash-list     # list available fixtures with per-fixture PASS/FAIL/TIME/SKIP
make dist               # cross-compile static binaries for all 6 platforms
make tidy               # go mod tidy + gofmt -s -w . + go vet ./...
```

Beyond the bash-5.3 fixture gate, the broader conformance matrix (engine
unit tests, POSIX-mode parity, the XCU/Oils/Austin/multi-shell differentials,
and the yash POSIX scoreboard) is driven via the `bashy dag` task runner — the
agent-first dogfood of the Makefile:

```sh
bashy dag suites.md -j8 -k          # whole conformance matrix in parallel (-k: don't halt on first failure)
bashy dag suites.md test-bash yash  # a subset of suites
bashy dag --list                    # what `make help` shows, as DAG targets (see DAG.md)
bashy dag --json test               # machine-readable envelope for an agent
```

`suites.md` and `DAG.md` are literate task files: each `###` heading is a
target with `Requires:`/`Sources:`/`Effects:` metadata, run in topological
order through the in-process shell. `suites.md` is the conformance matrix
(only `test-bash` is a hard 0/1 gate; the differentials are INFO probes);
`DAG.md` mirrors the Makefile's build/test/lint targets.

Under finer-grained `go`:

```sh
go build ./...
go test ./...
go test -run TestMain ./...
```

### Local-env PATH gotcha (wrapper shim)

If your `PATH` puts a wrapper shim in front of `sh` (some agentic dev tools
install one — `which sh` returns a `…/wrap/…/bin/sh`), Go tests that fork a
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
is the scoreboard. As of 2026-06 the bash-5.3 fixture suite is at **86 passing,
0 failing, 0 skipped (100% of 86 measured fixtures)** — so the active frontier
has shifted to the broader POSIX-conformance matrix in `suites.md` (the yash
POSIX scoreboard is the headline conformance-frontier metric there). A change
that flips a fixture FAIL → PASS without regressing anything else is worth
shipping; cleanup that doesn't move the count isn't the priority. Most flips
require a change in `../sh` (interp/expand/syntax) plus, sometimes, the CLI
glue here. Always re-read the live headline in `docs/TODO.md` rather than
trusting any count quoted here.

**Scoreboard reliability.** `make test-bash` is unreliable when a wrapper
shim shadows `sh` in `PATH` (see the gotcha above). To measure
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
- `followup-signal-death-message-format.md` — #25/#26 merged conformant (gating correct); byte-exact stderr WORDING is a tracked non-POSIX-mandated follow-up + how to handle it in the POSIX conformance suites.
- `scope-jobcontrol-fc-behaviors.md` — feasibility scoping of the remaining POSIX-mode job-control (#23–27,#49) + fc (#54–57) behaviors: TRACTABLE vs VERIFY vs CEILING, with the next two-issue fleet round.
- `plan-dynvar.md`, `plan-error-format-pass.md`, `plan-punted-builtins.md` — scoped sub-plans for specific clusters of fixture failures.
- `json-output.md` — bashy's opt-in `set --json` / `declare --json` structured-output extensions.
- `space-time-advisor.md` — the shipped space-time advisor: non-intrusive error-time hints (cwd/network/compute/disk + doomed-loop + network-fingerprint host memory) that steer agentic tools off doomed retries. Self-contained feature doc (dimensions, env vars, `bashy-advice-v1` JSON schema, scope/non-goals).
- `bash.md`, `agentic-extensions.md` — background references, not active plans.

POSIX-conformance frontier (the active layer now that bash-5.3 is 86/86 — driven via `suites.md` + `DAG.md`):

- `plan-posix-conformance.md` — plan of record for the POSIX-mode conformance push (the differential suites + yash scoreboard).
- `conformance-statement.md` — the standing conformance claim; `shell-conformance-comparison.md` / `cross-shell-conformance-baseline.md` — bashy vs other shells.
- `posix-mode-behaviors.md` — catalogued `--posix` behaviors; `builtin-vs-external-conformance.md` — builtin/external divergence notes.
- `posix-cert-handoff-runbook.md`, `posix-cert-preflight-status.md`, `fidelity-ceiling-assessment.md` — VSC-PCTS certification runbook + status + the hard-ceiling assessment.

Per-fixture cluster analyses + blocker ledgers (snapshots — diff line-counts and PASS/FAIL claims in them are dated, re-measure before trusting):

- `ARITH-ANALYSIS.md`, `ARRAY-ANALYSIS.md`, `ASSOC-ANALYSIS.md`, `DBG-SUPPORT-ANALYSIS.md`, `NAMEREF-ANALYSIS.md`, `NEWEXP-ANALYSIS.md` — failure-cluster breakdowns for the named fixtures.
- `NEWEXP-RESIDUE-R2.md`, `ERRORS-ANALYSIS-R2.md` — round-2 residue analyses.
- `ERRORS-BLOCKERS.md`, `HEREDOC-BLOCKERS.md`, `HISTORY-BLOCKERS.md`, `QUOTEARRAY-BLOCKERS.md`, `VARENV-BLOCKERS.md` — per-fixture blocker ledgers.

Weave-round verification + retro reports (historical, not load-bearing):

- `QA-REPORT-R10.md`, `JUDGE-REPORT-R6.md`, `JUDGE-REPORT-R7.md`, `SPRINT-R10-RETRO-DRAFT.md`.

## Skills

`skills/` holds the tier-2 **workspace** agentic skills bashy ships (the
userland is tier 1, clusters tier 3). Each is a self-contained Anthropic skill
(`SKILL.md` actionable checklist + optional `reference.md` deep companion),
brand-neutral and driven by bashy's own tools:

- `skills/conductor/` — drive a fleet of agent CLIs to a verified goal over
  `bashy sprint` + `bashy weave` (decompose → isolate → gate → converge, loop
  until a verifier passes); TDD-at-fleet-scale is the canonical mode.

## Plans

Always save a copy of all implementation plans in `docs/`. Use a descriptive
filename (e.g. `docs/plan-feature-name.md`).

## Third-Party Libraries

- **Permissive licenses only**: MIT, BSD, Apache 2.0, or equivalent. No GPL/LGPL.
- **Pure Go only**: no CGo, no C dependencies (the `cc` invocation in
  `test-bash-helpers` builds Bash's own test helpers, not bashy).
