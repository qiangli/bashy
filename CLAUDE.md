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
  cat/ls/grep/… , the `ast` code-intel command (ast symbols/search/refs/map/query),
  the `graph` verb's code-knowledge-graph read subcommands (graph build/stats/neighbors/impact/path/hotspots/query,
  gfy-backed, model-free), and its knowledge-graph CONTRIBUTION subcommands
  (graph note/link/observe/forget write · graph recall/notes/pitfalls read —
  a durable, shared, per-repo "agentic wiki" agents enrich; append-only store at the repo root),
  in-process across
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
now a structural cmd/ split.)

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
  `NewManagedOllamaCmd` — isolated daemon, own port/models; plus `bashy run`
  (result envelope), `commands` (command-surface lister), `doctor` (environment
  self-diagnostic), `act-runner`, `loom`, `zot`, `seaweedfs`, `kopia`). Imported
  only by `cmd/bashy`, so the lean `bash` binary never links any of it. The
  coreutils userland also carries the agentic tools `fetch` (REST/URL client),
  `tokens` (LLM token counter), and `clip` (system clipboard) — see
  `docs/slash-command-priorart-survey.md`.
  - **The agent-facing envelope verbs** live beside it as one file each, and are
    the intended entry points for an agentic tool driving bashy (see
    `skills/bashy/`): `context.go` (`bashy context --json` — machine-readable
    host/session/capability snapshot, the *first* call an agent makes), `run.go`
    (result envelope), `dryrun.go` + `check.go` (`--dry-run` / script check
    before execution), `verify.go`, `commands.go` + `atlas.go` (the Command
    Atlas lister), `doctor.go` (environment self-diagnostic), `nudge.go`,
    `installagent.go` (`bashy install-agent` — point an agent CLI's shell at
    bashy), `git.go`/`git_verbs.go`, `self.go`. Adding a verb means touching its
    file **and** its atlas entry — the coverage tests and the CI e2e dispatch
    gate both fail otherwise.
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
    weave "$@"; }`, …). Shadowing policy: native verbs (weave/sprint/dag/run/
    commands/doctor/schedule/secrets/skills/kb) + identical drop-in passthroughs
    (gh/act/rclone/podman/ollama/loom/zot/seaweedfs/kopia/mirror)
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
    - `-tags bashy_engines` (`engines_{full,stub}.go`) — the *in-process linked*
      container/LLM engines `bashy podman`/`ollama` (cgo + btrfs/MLX). Always
      excluded on Windows. In the default lean build the stub does NOT error —
      per the settled dispatch ladder (Tier 0 shell → 1 pure-Go userland →
      2 managed engine, **exec'd, never linked** → 3 PATH fallback → 4 mesh
      delegate) it falls through to **Tier 3**: resolve a host/binmgr-cached
      `podman`/`ollama` and exec it transparently (no rebuild), or, if none is
      found, point to install/a paired host node — so a `bashy commands` verb
      always runs without a rebuild step.
    - `-tags bashy_obs` (`obs_{full,stub}.go`) — the observability stack
      `bashy otel` (OpenTelemetry Collector + VictoriaMetrics/Logs + Jaeger +
      Perses + k8s/aws, **193 MB**).

    `make build` = lean; `make build-host` (= `BASHY_ENGINES=1 BASHY_OBS=1`,
    pulling in the embed blobs too) = full unix host. Rule of thumb: a worker
    essential that's pure-Go + cross-platform is **core** (compiled in); a heavy
    or cgo host service is **ext** (build-tag, or binmgr download-on-demand).

## Module wiring

`go.mod` requires three flat-sibling deps, resolved by `replace`:

```
replace mvdan.cc/sh/v3               => ../sh
replace github.com/qiangli/coreutils => ../coreutils
replace github.com/ergochat/readline => ../readline
```

`../sh` is the interpreter engine; `../coreutils` is the AgentOS hub that
supplies the pure-Go userland + code-intel verbs the `bashy` binary injects (only
`agentos.go` imports it); `../readline` is the ergochat/readline fork the
interactive loop uses (the module path keeps the upstream name — the flat-layout
convention is about the sibling dir, not the module string). In a parent
monorepo all three are submodules. In
a standalone clone, run `./scripts/bootstrap-siblings.sh` — it clones each
sibling next to this repo at the SHAs pinned in
`.sibling-pins` (and leaves any submodule mounts alone). CI does the
same before building. coreutils itself replaces `../sh`, which resolves to the
same flat sibling. Keep the sibling SHAs coordinated; a parent monorepo's
sync tooling auto-bumps `.sibling-pins`. (go.mod also carries further
`../coreutils/...`-internal replaces for the embedded podman/ollama/otel
engines — those ride the coreutils pin, not `.sibling-pins`.)

**Bumping a sibling means bumping `.sibling-pins` in the same breath.**
`.sibling-pins` is the only sibling source CI ever sees — it has no umbrella, so
it clones each sibling at the pinned SHA. A local build **cannot** catch a stale
pin: the umbrella mounts the live siblings as submodules, so the pins are never
consulted here. The build passes locally against the new sibling while CI builds
the old one and fails with a mystifying `no required module provides package` for
code that plainly exists. (That is exactly how a stale coreutils pin broke every
build for a dozen commits — the packages CI couldn't find had been added to
coreutils *after* the pinned SHA.)

Because push time is the only honest moment to notice, `scripts/hooks/pre-push`
refuses a push while a pin disagrees with its sibling's HEAD. It is a no-op in a
standalone clone (no siblings to compare), names the drifting sibling, and is
bypassable with `git push --no-verify`. Install it with `make hooks` — or just
run `./scripts/bootstrap-siblings.sh`, which now sets `core.hooksPath` for you.
To resync after bumping a sibling: `./scripts/update-sibling-pins.sh`, then
commit the pins with the change that needs them. Push the sibling to its own
origin too — CI clones the pin from GitHub, so a SHA that exists only on your
machine fails there as well.

## Build / test / lint

```sh
make build              # -> bin/bash (pure drop-in, cmd/bash) + bin/bashy (AgentOS, cmd/bashy) — two independent binaries
make build-bash         # only bin/bash — all the conformance harness needs (skips the embed-heavy bashy build)
make build-host         # full unix host build (= BASHY_ENGINES=1 BASHY_OBS=1 + embed blobs)
make install            # install to $DHNT_BIN_DIR (default ~/.local/bin)
make test               # go test ./...
make test-bash          # drive bin/bash against bash's own 5.3 test suite (serial)
make test-bash-parallel # same suite fanned out across cores — the canonical 86/86 gate
make test-bash-list     # list available fixtures with per-fixture PASS/FAIL/TIME/SKIP
make test-yash          # yash POSIX (-p) scoreboard — the headline conformance-frontier metric
make test-yash-list     # print the current bashy-specific yash failure list
make test-zsh           # zsh-own-suite Tier-0 scoreboard (tools/ztst runner; INFO metric, not a gate)
make test-uutils        # REFUSES native host execution: use only the contained runner (OOM/root-walk landmines)
make dist               # cross-compile static binaries for all 6 platforms
make tidy               # go mod tidy + gofmt -s -w . + go vet ./...
make help               # every target with its `## ` doc line
```

**Running a single test.** Two axes, depending on what you're iterating on:

```sh
make test-bash TESTS="comsub varenv"      # only those bash-5.3 fixtures (also honored by test-bash-parallel)
make test-bash-run TESTS="comsub"         # the fixture loop WITHOUT rebuilding bin/bash
go test -run TestPromptExpand ./internal/cli   # one Go test
go test -run TestDoctor -v ./internal/agentos
```

`TESTS=` is the fast inner loop for conformance work — the full serial suite is
minutes, one fixture is seconds.

### Conformance-suite host safety

Never run the full uutils suite natively. A 2026-07-24 run triggered unbounded
reads from `/dev/zero`/`/dev/random` and recursive `--preserve-root` bypasses
that walked root-equivalent paths. `scripts/uutils-scoreboard.sh` now refuses
host execution and quarantines the known cases. Resume only in a disposable,
non-root container/VM with hard memory, PID, and wall-time limits and no
host-root/home mount. The cross-repository policy and exact cases are recorded
in `../docs/conformance-test-landmines.md`.

Beyond the bash-5.3 fixture gate, the broader conformance matrix (engine
unit tests, POSIX-mode parity, the XCU/Oils/Austin/multi-shell differentials,
and the yash POSIX scoreboard) is driven via the `bashy dag` task runner — the
agent-first dogfood of the Makefile:

```sh
./bashy dag build                   # fresh checkout bootstrap: builds bin/bashy if needed
./bashy dag install                 # install into $DHNT_BIN_DIR (default ~/.local/bin)
bashy dag suites.md -j8 -k          # whole conformance matrix in parallel (-k: don't halt on first failure)
bashy dag suites.md test-bash yash  # a subset of suites
bashy dag --list                    # what `make help` shows, as DAG targets (see dag.md)
bashy dag --json test               # machine-readable envelope for an agent
```

`./bashy` (repo root) is a POSIX-sh bootstrap: it builds `bin/bashy` on first
use (preferring an already-installed `bashy` to compile itself) and then execs
it, so a fresh checkout can run `./bashy dag …` with nothing but Go on the box.
Once `make install` has run, drop the `./` and use the PATH binary.

`suites.md` and `dag.md` are literate task files: each `###` heading is a
target with `Requires:`/`Sources:`/`Effects:` metadata, run in topological
order through the in-process shell. `suites.md` is the conformance matrix
(only `test-bash` is a hard 0/1 gate; the differentials are INFO probes);
`dag.md` mirrors the Makefile's build/test/lint targets and adds the chunked /
fleet / container conformance lanes (`test-bash-chunks`, `test-bash-chunks-fleet`,
`test-bash-chunks-container`, `yash-chunks`) that the Makefile has no equivalent
for. **The file is `dag.md`, lowercase** — `DAG.md` only resolves on a
case-insensitive filesystem (macOS) and breaks on Linux/CI.
Inside DAG target bodies, use `"$BASHY" ...` for recursive bashy calls. Mirroring
GNU Bash's `BASH`/`BASH_ARGV0` split, `bashy dag` injects `BASHY`/`BASHY_EXE`
as the resolved executable path and `BASHY_ARGV0` as the raw argv0 string, so
targets do not drift to a stale PATH binary.

Under finer-grained `go`:

```sh
go build ./...
go test ./...
go test -run TestMain ./...
```

### Before pushing (what CI will run)

`.github/workflows/test.yml` runs a **3-OS matrix (ubuntu / macOS / windows)**,
and the Windows leg is the one that catches things a local unix run cannot:

- build + vet + `go test ./internal/agentos` on all three (Windows skips
  `internal/cli` — its readline / forced-interactive tests hang without a PTY);
- an **e2e dispatch gate** — `go test -tags e2e -run
  TestE2EAllListedCommandsDispatch ./internal/agentos` asserts every verb
  `bashy commands` advertises actually runs on that OS. Adding a verb without an
  atlas entry or a working stub fails here;
- a **cross-build of the lean `cmd/bashy` for all 6 release platforms** with
  `CGO_ENABLED=0`.

So before pushing, at minimum cross-build for Windows (`CGO_ENABLED=0
GOOS=windows GOARCH=amd64 go build ./cmd/bashy`) plus `go test ./...`. Running
the workflow under `bashy act` does **not** cover this — act is Linux-only.

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

**Scoreboard reliability.** There is exactly **one fixture runner**:
`tools/bash53suite`. `make test-bash`, `make test-bash-parallel` and every
`bashy dag` chunk target drive that same binary — which is what makes
"chunked == serial" a checkable claim. (Until 2026-07-12 the Makefile
implemented a *second* runner in shell whose watchdog silently failed to kill a
wedged fixture; that hung CI for 20 minutes a run, and `continue-on-error: true`
reported it green while the gate went unmeasured for ~10 merges. Do not
reintroduce a second runner.)

The harness owns what the shell loop used to bolt on: the per-fixture transforms
(`expect`-line filtering, `cat -v` for control-char fixtures like `printf`), a
4 GB memory cap, a per-fixture timeout that always terminates, and a **private
per-run tree** — its own copy of the corpus plus its own `HOME` and `TMPDIR`. That
last part is load-bearing: the C helpers (`recho`/`zecho`/`xcase`) are built *into*
the fixture tree, so a shared tree lets a container run's ELF binaries poison a
native run (and vice versa) — measured at 47/86 vs 77/86 on the same container,
decided only by who built the helpers last. Private `TMPDIR` likewise kills the
`histexpand`/`history` cross-chunk race (they share `$TMPDIR/newhistory`).

Two things still bite:

- **A wrapper shim shadowing `sh` in `PATH`** (see the gotcha above) — run with a
  clean `PATH` (`PATH=/bin:/usr/bin:$(dirname $(which go))`).
- **A missing `external/bash-5.3` symlink false-*passes*** — the fixtures simply
  aren't there to run. The CI gate refuses a run with zero PASS lines for exactly
  this reason.

`BASH_TEST_SKIP` (and the harness's `-skip`) still exists for local iteration, but
**CI refuses any skipped fixture** (`scripts/ci-bash53-gate.sh`): a skip is silent
coverage loss, and the ratchet cannot see it (a SKIP is not a FAIL). That is how
this gate failed before — `coproc`, `jobs` and `trap` were skipped *because they
hung*, so CI stayed green while three fixtures went unmeasured. Nothing is skipped
today; all 86 run.

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

- `philosophy.md` — **the thesis: LOCAL FIRST.** "bashy is all an agent needs" — the whole
  SDLC loop (issue → weave → gate → judge → dag) closes on ONE machine with NO network,
  and that claim is *enforced*, not asserted: `pkg/atlas/localfirst_test.go` fails the
  build if a loop verb starts declaring the `net` effect. The air-gapped room is a TEST,
  not a market (if it works there it works on the plane, in the outage, and on hotel
  wifi). Three pillars (compatibility → capability → agency), six venues (venue 1 is a
  complete product, not a fallback), and what the philosophy FORBIDS. Read before any
  feature that reaches for a hosted service.
- `TODO.md` — phase checklist + current PASS/FAIL/SKIP headline. Always read first.
- `report-bash53-test-status.md` — per-fixture status snapshot from the bash 5.3 suite.
- `handoff-bashy-2026-06.md` — most recent session-handoff notes (read when picking up cold).
- `bash-gap-analysis.md` — ungated bash semantics gap analysis behind the failing fixtures.
- `plan-bashy-drop-in.md` / `plan-cmd-bashy.md` / `plan-bash53-roadmap-agentic.md` — phase plans; each phase lands as a checkbox in `TODO.md`.
- `followup-signal-death-message-format.md` — #25/#26 merged conformant (gating correct); byte-exact stderr WORDING is a tracked non-POSIX-mandated follow-up + how to handle it in the POSIX conformance suites.
- `scope-jobcontrol-fc-behaviors.md` — feasibility scoping of the remaining POSIX-mode job-control (#23–27,#49) + fc (#54–57) behaviors: TRACTABLE vs VERIFY vs CEILING, with the next two-issue fleet round.
- `plan-dynvar.md`, `plan-error-format-pass.md`, `plan-punted-builtins.md` — scoped sub-plans for specific clusters of fixture failures.
- `json-output.md` — bashy's opt-in `set --json` / `declare --json` structured-output extensions.
- `agent-bands-and-nicknames.md` — the shipped **band** (L1–L4 capability peg, normalized across providers — a vendor's own tier ladder is never mapped positionally) + **nickname** system on `bashy agents`/`models`. Bands live on the model and are inherited by the agent; `--min-band N` selects a roster (`bashy meet start --min-band 3` seats its own table and reports who it skipped). Canonical model names are version-explicit (`opus4.8`) and the family name (`opus`) is a *derived* alias that re-points itself on release — so a record never rots. Nicknames are assigned deterministically from the binding (same agent, same name, every host). Rules: speak the alias, record the address; a binding is canonicalized however it was spelled; a derived name never shadows a declared one. Read before any fleet-registry / agent-selection / routing work.
- `command-atlas.md` — the Command Atlas: the multi-axis agent-facing catalog of the whole command surface (classical group + execution tier + capability + idiom axes). Tables live in `coreutils/pkg/atlas` (coverage-test-ratcheted against `tool.Names()`); the bashy merge layer is `internal/agentos/atlas.go`; views via `bashy commands --view tier|group|capabilities`, `--tier/--group/--cap` filters, `--idioms`, `--atlas` (`bashy-atlas-v1`). Adding a verb/tool = add its atlas entry (the tests name what you forgot).
- `space-time-advisor.md` — the shipped space-time advisor: non-intrusive error-time hints (cwd/network/compute/disk + doomed-loop + network-fingerprint host memory) that steer agentic tools off doomed retries. Self-contained feature doc (dimensions, env vars, `bashy-advice-v1` JSON schema, scope/non-goals).
- `one-agent-control.md` — **the one control surface** every command that drives an agent CLI now steers through (`invoke` · `weave` · `meet` · `foreman`). `chat.Session` (Start/Say/WaitIdle/Turn) is the primitive — *Invoke is a question, Session is a conversation* — and it lives in `chat` because that is where `agentChildEnv` (secret scrub · single granted API key · shell-forcing · principal identity) lives. `agentpty` owns the wire (`TextFrame` = a sentence typed; `VerbatimFrame` = a keystroke), collapsing three divergent copies of one protocol. Why `meet --steerable` is a flag and not a default (a live turn under a THIRD-PARTY CLI has no boundary — it ends on silence, so it pays a quiet period out and a TUI startup in). **A tool that declares `events_arg:` escapes that**: it reports `turn.end` and bashy believes it, because that is a fact the agent asserted rather than a silence bashy interpreted — today only `ycode` does (see `first-party-harness.md`). Also: `foreman interrupt` (ESC as a real keystroke) — a queued message never reaches an agent stuck in a tool loop, because it reads its queue only between turns and that turn is never going to end. Read before any steering / `say` / `tell` / agent-launch work.
- `chat-interactive-launcher.md` — **`bashy chat` as the governed front door** for launching a third-party agent CLI *interactively*: the tool's NATIVE UX (agentpty's raw-mode local-TTY passthrough, not a bashy REPL) but with the fleet-selected model, full `agentChildEnv` governance, and a live-sessions registry (`~/.bashy/sessions/`) that makes the launched agent ADDRESSABLE — `chat sessions`/`steer`/`interrupt`/`attach`, later coach/meet. Selection: `--agent NICK` (specific) or `--band N`/`--tool T` (any operable one, reusing `SeatByBand`). `invoke` stays the one-shot (*Invoke is a question, Session is a conversation* — finally implemented). ycode is special-cased (already bashy-native → just launches it with the resolved `--model`). Companion to `one-agent-control.md`. Read before any interactive-launch / session-registry / chat-mode work.
- `absence-of-evidence.md` — **the day's real product, and the codebase's characteristic failure.** SEVEN instances in one day of ONE shape: *a success state reached by the absence of evidence.* Declared fields nothing writes (`ConversationMessage.Usage`, `ExemptFromMasking`, `StreamOptions`, `SessionTotalCost`, 3 config fields), caps that bind and exit 0, a pricing fallback that bills an unknown model at Claude's rate. Every one produced a PLAUSIBLE ANSWER THAT WAS NOT TRUE, and four of them nearly got recorded as facts about a MODEL. Also: the four times my own instruments lied (`cmd | head && echo OK` chains off head's exit; `rm` on a receiver's open file; a bad `pgrep` pattern; an OTLP receiver silently dropping span events). Read before trusting any green check.
- `observability.md` — the shipped OTel plane. bashy could RUN a collector (`bashy otel`) and fed it NOTHING — it was the one tier of the whole stack missing from the umbrella's `service.name` set. Two primitives, chosen from what six hours of debugging could not see: **Provenance** (a value next to WHERE IT CAME FROM — the only bug caught by a signal was caught by `from_provider=false`) and **BoundHit** (a limit records when it BINDS — especially when the run recovers). Plus a span per command at the ExecHandler chokepoint, including the EXIT CODE. Stack trimmed 286 MB → 109 MB (−61%) by going Victoria-only: jaeger (2,240 deps) → VictoriaTraces, perses (1,478) → vmui, collector (833) → three proxy map entries, prometheus (556) → VictoriaMetrics. Pure standard OTEL env vars; unset endpoint is a total no-op; `cmd/bash` links none of it.
- `audit-log.md` — the shipped compliance audit trail: a tamper-evident, hash-chained, secret-redacted record of every dispatched command with agent attribution and Command-Atlas effects (`bashy-audit-v1`; NIST AU-3/AU-9). Opt-in via `BASHY_AUDIT`, off by default, never in `cmd/bash` / `--posix`. Read side is `bashy audit {status,tail,verify,export,path}`; core is `coreutils/pkg/policy/audit`, the ExecHandler middleware is `internal/agentos/audit.go`. Records; does not block (policy engine) or contain (OS sandbox) — the un-bypassable record of the agentic+interactive command path, composes with auditd/EDR. Deferred: OTel export, signed checkpoints, gitleaks-grade redactor.
- `fips-140.md` — the shipped FIPS 140-3 build mode: `make build-fips` (`GOFIPS140=v1.0.0`) builds both binaries against the Go Cryptographic Module (CMVP #5247); pure-Go, no cgo/BoringCrypto. Use `GODEBUG=fips140=on` (the build-fips default — keeps `md5sum` working), NOT `fips140=only` (rejects MD5) for a general shell. State surfaced in `bashy doctor` and `bashy context --json` (`runtime.fips140`). A FIPS-built `bin/bash` still passes 86/86. Pairs with the audit log for the FedRAMP/CMMC procurement story.
- `bash.md`, `agentic-extensions.md` — background references, not active plans.

POSIX-conformance frontier (the active layer now that bash-5.3 is 86/86 — driven via `suites.md` + `dag.md`):

- `plan-posix-conformance.md` — plan of record for the POSIX-mode conformance push (the differential suites + yash scoreboard).
- `conformance-statement.md` — the standing conformance claim; `shell-conformance-comparison.md` / `cross-shell-conformance-baseline.md` — bashy vs other shells.
- `posix-mode-behaviors.md` — catalogued `--posix` behaviors; `builtin-vs-external-conformance.md` — builtin/external divergence notes.
- `posix-cert-handoff-runbook.md`, `posix-cert-preflight-status.md`, `fidelity-ceiling-assessment.md` — VSC-PCTS certification runbook + status + the hard-ceiling assessment.
- `yash-conformance-gap.md` — the yash-scoreboard failure analysis behind the headline number in `docs/TODO.md`.
- `zsh-scoreboard.md` — the zsh Tier-0 own-suite baseline (`make test-zsh`, `tools/ztst` runner); INFO metric, not a gate.
- `chunked-fleet-conformance-plan.md` — the chunked/fleet/container conformance lanes in `dag.md` (`test-bash-chunks*`, `yash-chunks*`): chunk count is a corpus property pinned in a committed manifest, and the authoritative run stays single-host + unchunked (`test-bash` 86/86 serial is the release gate) — campaign mode never speaks for it.
- `ci-failure-autorepair-plan.md` + `config/ci-failure-fixer.env` + `scripts/ci-failure-{router,fixer,gate}.sh` — the `.github/workflows/ci-failure-report.yml` lane that routes a CI failure to a **fixer** run (the band-selected agent that repairs one failing gate — a lighter role than the SDLC `conductor`, which is the escalation target for a fix that needs orchestration).
- `bashy-v1.0.0-readiness.md` — the release-readiness ledger.
- `agent-adoption/matrix.md` — which agentic CLIs are verified running on bashy as their shell (the `force-agent-shell` skill's evidence base).
- `first-party-harness.md` — **why ycode is in the fleet, and what it actually buys.** All four "still owed" items shipped 2026-07-14. The differentiator is NOT that it wins a bake-off (it lost — slowest, most code): it is the **event channel**. `--events` emits `turn.start`/`tool.call`/`turn.end` as NDJSON on both the one-shot and TUI paths, so a turn's end is a FACT THE AGENT REPORTS rather than a silence bashy interprets (`WaitIdle`, 25s). `turn.end.text` equals `--print` stdout exactly. Not yet reached: server mode (the agent loop lives in the server process, which never sees the client's `--events`). Read before any harness-selection or `chat.Session` work.
- `band-ladder.md` — **the L1–L4 ladder across every provider**, with the two open questions now ANSWERED by running both as conductors: `gemini3.1` demoted L3→L2 (9.4× repeat ratio, never converged — a coder, not a lead; confound recorded), `deepseek-v4-pro` CONFIRMED L3 (1.2×, decomposed and delegated unprompted). The loop metric — total tool calls ÷ distinct — is the cheapest conductor health check there is. Read before any band re-peg or conductor selection.
- `fleet-live-verification.md` — `bashy agents verify --live`: why a STRUCTURAL check (both halves of a binding resolve in the catalog) is not evidence that an agent can speak, and how five dead bindings hid behind one that looked healthy. The origin of "a verifier that passes on the ABSENCE of a known failure is not a verifier."
- `harness-ab-deepseek.md` — **the three-harness A/B** (ycode vs opencode vs aider, one model, one task, one gate). All three converge; the differences were in the HARNESS, and two were ours. Headline finding: **all three exit 0 when they fail** — a harness's exit code carries no information, so run the gate. Also why aider is retired from the API-key lane (it cannot discover the files a task needs — architecture, not quality) and why opencode is KEPT (the cross-check against a first-party bug). Read before any harness-selection or fleet-routing decision.

Per-fixture cluster analyses + blocker ledgers (snapshots — diff line-counts and PASS/FAIL claims in them are dated, re-measure before trusting):

- `ARITH-ANALYSIS.md`, `ARRAY-ANALYSIS.md`, `ASSOC-ANALYSIS.md`, `DBG-SUPPORT-ANALYSIS.md`, `NAMEREF-ANALYSIS.md`, `NEWEXP-ANALYSIS.md` — failure-cluster breakdowns for the named fixtures.
- `NEWEXP-RESIDUE-R2.md`, `ERRORS-ANALYSIS-R2.md` — round-2 residue analyses.
- `ERRORS-BLOCKERS.md`, `HEREDOC-BLOCKERS.md`, `HISTORY-BLOCKERS.md`, `QUOTEARRAY-BLOCKERS.md`, `VARENV-BLOCKERS.md` — per-fixture blocker ledgers.

Weave-round verification + retro reports (historical, not load-bearing):

- `QA-REPORT-R10.md`, `JUDGE-REPORT-R6.md`, `JUDGE-REPORT-R7.md`, `SPRINT-R10-RETRO-DRAFT.md`.

## Skills

`skills/` holds the tier-2 **workspace** agentic skills bashy ships (the
userland is tier 1, clusters tier 3). They are **compiled into the `bashy`
binary** via the `//go:embed` directive in `skills/embed.go` (surfaced by
`bashy skills`), so adding a skill means dropping its directory here AND adding
it to that directive. Each is a self-contained Anthropic skill
(`SKILL.md` actionable checklist + optional `reference.md` deep companion),
brand-neutral and driven by bashy's own tools:

- `skills/bashy/` — how to drive bashy itself as an agent (start with
  `bashy context --json`; dry-run/check/run envelopes; code-intel verbs).
- `skills/conductor/` — drive a fleet of agent CLIs to a verified goal over
  `bashy sprint` + `bashy weave` (decompose → isolate → gate → converge, loop
  until a verifier passes); TDD-at-fleet-scale is the canonical mode.
- `skills/go-repo-health/` — the reference dual-bundle skill (`SKILL.md` +
  `skill.dhnt`): attested build-ok ∧ tests-green gate for a Go repo.
- `skills/knowledge-transfer/` — agent-to-agent knowledge transfer via
  `bashy kb`: the MENTOR loop (distill private memory / in-context recall
  into reconciled candidate pages; select durable+team-relevant+non-derivable;
  redaction gate; `xfer:<source>` idempotence tags; procedures route to
  `skills learn`, prose to kb) and the MENTEE loop (search-before-task →
  validate-through-use → pointers-not-copies localization). Hard rules:
  transferred ≠ validated (a second agent promotes), kb reads foreign stores
  but never writes them.
- `skills/force-agent-shell/` — attested check that agentic CLIs route their
  shell commands through bashy (so the pure-Go userland, the advisor, and OTel
  apply to everything an agent runs). Run as a convergence gate before an
  unattended fleet run: `bashy skills run force-agent-shell` (exit 0 iff the
  contract holds); wiring is `bashy install-agent <agent>` (`--check` to verify).

## Plans

Always save a copy of all implementation plans in `docs/`. Use a descriptive
filename (e.g. `docs/plan-feature-name.md`).

## Third-Party Libraries

Full policy: `docs/licensing-supply-chain-policy.md`. In brief:

- **Compiled-in / embedded / linked / vendored → permissive only**: MIT, BSD,
  Apache 2.0. No GPL/LGPL/MPL/SSPL/BSL/proprietary — nothing whose license could
  propagate. Record each in `THIRD_PARTY_LICENSES`.
- **Pure Go only** for the core: no CGo, no C deps (the `cc` in
  `test-bash-helpers` builds Bash's own test helpers, not bashy).
- **Runtime download + exec ≠ bundling**: tools bashy downloads and runs as
  separate processes (podman/ollama/gh/loom/act/…, and fetched test suites) are
  not bundled — separate programs on their own licenses, no propagation. Prefer
  permissive anyway.
- **Required + no permissive substitute → build from permissive source** via the
  self-provisioning toolchain (`bashy go`/`cmake`/`clang`), in CI or on demand —
  never ship a non-permissive prebuilt.
