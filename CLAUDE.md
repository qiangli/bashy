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
  (measured 2026-08-05 on darwin/arm64: **6.2 MB vs. bashy's 81 MB**; see
  §Binary size for where the difference goes). **The compliance harness drives `bin/bash`, so
  the conformance work measures this pure drop-in.**
- **`bashy`** (`cmd/bashy`) — the **AgentOS system shell**: the same shell core
  plus the coreutils `shell.Handler()` ExecHandler (pure-Go userland
  cat/ls/grep/… , the `ast` code-intel command (ast symbols/search/refs/map/query),
  the `graph` verb's code-knowledge-graph read subcommands (graph build/stats/neighbors/impact/path/hotspots/query,
  gfy-backed, model-free), and its knowledge-graph CONTRIBUTION subcommands
  (graph note/link/observe/forget write · graph recall/notes/pitfalls read —
  a durable, shared, per-repo "agentic wiki" agents enrich; append-only store at the repo root),
  and its EXECUTION subcommands (graph history/space/reached read · graph learn
  writes what was observed into kb as CANDIDATE pages · graph evidence resolves a
  page's pointer back to the raw records — the agentic replacement for the
  interactive-only `history` builtin; see `docs/agentic-history-and-space-graph.md`
  and, for the layer model, `../docs/knowledge-substrate-reconciliation.md`),
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

    The dispatcher now carries ~90 verbs, so **do not read this list as the
    surface** — `agentos.go`'s `switch` is the dispatch truth and
    `bashy commands --atlas` is the catalog. Orchestration/agent-fleet verbs
    landed as their own files the same way: `coord.go` (`bashy claim` — a WRITE
    is refused while another agent holds the project; enforced in the SHELL
    because no document is mandatory across agent CLIs), `messageboard.go`
    (`bashy mb` over `coreutils/pkg/bus`; `wireMessageBoard` has a test
    asserting every hook is non-nil, because an unwired seam looks finished),
    `steward.go`/`steward_meet.go`/`steward_mediator.go` (`bashy steward
    start|stop` — putting an agent ON the seat, not just describing it),
    `kbrecall.go` (`bashy kb recall`, mounted here rather than in `pkg/kb` to
    keep that package an import leaf), `exechist.go`, `release.go`,
    `agents.go`, `shellsession.go` + `session/` (the live-session socket, with
    peer-credential checks per OS).
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
    commands/doctor/schedule/secrets/ask/skills/kb) + identical drop-in passthroughs
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
    (~259 MB with blobs); `bin/bash` stays ~6 MB.

    **Binary size — measured 2026-08-05, darwin/arm64, `make install` (meetspa
    embed only, no engine blobs): `bashy` 81 MB, `bash` 6.2 MB.** The older
    "~121 MB unix / ~47 MB Windows" figure below has not been re-measured since;
    treat it as stale until `make dist` confirms it. Where the 75 MB goes, by
    segment: `__rodata` 28.9 MB (vs 0.1 MB in `bash`), `__text` 20.7 MB,
    `__gopclntab` 16.9 MB, `__DATA_CONST` 12.1 MB.

    **The single largest item is one dependency: ~21.9 MB of tree-sitter grammar
    tables.** A probe importing `coreutils/pkg/treesitter` is 24.3 MB against a
    2.4 MB hello-world baseline. **Dead-code elimination does not help**, and the
    obvious fix is a no-op: a probe referencing only `grammars.GoLanguage` builds
    to the same 24.3 MB as one referencing all nine, because `//go:embed
    grammar_blobs/*.bin` pulls the whole directory into one `embed.FS` regardless
    of which loaders are called. Trimming `pkg/treesitter/languages.go` would save
    zero bytes. Upstream already ships both tiers as build tags —
    `-tags grammar_set_core` (100 langs) is 17.9 MB, `-tags grammar_blobs_external`
    (read from disk) is 4.0 MB. Adopting them is a tracked item in
    `docs/TODO.md` §Tree-sitter grammar tiering. **Verify any size claim by
    measuring the binary, not by counting references.**
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

`go.mod` requires four flat-sibling deps, resolved by `replace`:

```
replace mvdan.cc/sh/v3               => ../sh
replace github.com/qiangli/coreutils => ../coreutils
replace github.com/ergochat/readline => ../readline
replace github.com/filebrowser/filebrowser/v2 => ../filebrowser
```

`../sh` is the interpreter engine; `../coreutils` is the AgentOS hub that
supplies the pure-Go userland + code-intel verbs the `bashy` binary injects (only
`agentos.go` imports it); `../readline` is the ergochat/readline fork the
interactive loop uses (the module path keeps the upstream name — the flat-layout
convention is about the sibling dir, not the module string); and `../filebrowser`
is the maintained qiangli/filebrowser fork used by the AgentOS file-management
surface. In a parent monorepo all four are submodules. In
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
make install            # install to $DHNT_BIN_DIR (default ~/.local/bin) — installs the .real pair too
make build-fips         # both binaries against the Go FIPS 140-3 module (GOFIPS140) — see docs/fips-140.md
make test               # scripts/test-build-fail-closed.sh, then go test ./...
make test-bash          # drive bin/bash against bash's own 5.3 test suite (serial)
make test-bash-parallel # native host diagnostic, fanned out across cores
make test-bash-container # authoritative hermetic 86/86 gate (self-contained Linux image)
make test-bash-list     # list available fixtures with per-fixture PASS/FAIL/TIME/SKIP
make test-yash          # yash POSIX (-p) scoreboard — the headline conformance-frontier metric
make test-yash-list     # print the current bashy-specific yash failure list
make test-zsh           # zsh-own-suite Tier-0 scoreboard (tools/ztst runner; INFO metric, not a gate)
make test-uutils        # REFUSES native host execution: use only the contained runner (OOM/root-walk landmines)
make test-uutils-safety # the only bounded uutils harness validation that may run natively
make dist               # cross-compile static binaries for all 6 platforms (pure Go, no siglaunch — see below)
make smoke-chat AGENT=… # governed-launcher contract smoke (INFO, SKIPs without an agent or pty)
make hooks              # install scripts/hooks/pre-push (the .sibling-pins drift guard)
make tidy               # go mod tidy + gofmt -s -w . + go vet ./...
make help               # every target with its `## ` doc line
```

### The unix binaries are a C launcher over the Go binary

On linux/darwin, `make build` / `build-bash` / `build-bashy` / `build-fips` /
`install` emit **two files per shell**: `bin/bash.real` (the Go program) and
`bin/bash` (a small native C launcher compiled from `native/siglaunch.c.in` with
`cc`, which execs it). Same for `bashy`/`bashy.real`. The launcher exists because
the Go runtime resets most inherited `SIG_IGN` dispositions before `main`, and a
POSIX shell must remember them forever — siglaunch snapshots them pre-Go and
passes the names through the interpreter's sideband. Added 2026-08-07 for the
POSIX-cert startup-signal behavior — its regression test is
`internal/cli/signal_tp714_fault_unix_test.go` (plus `native/siglaunch_test.go`).

Consequences: `go build -o bin/bash ./cmd/bash` **overwrites the launcher with the
Go binary** and silently loses the snapshot — always go through the Makefile. The
harness, `make install`, and the installer all know about the `.real` pair.
Windows and every `make dist` cross-compile are plain pure-Go binaries with no
launcher (`CGO_ENABLED=0`), so this is a host-build shape, not a shipped-artifact
one.

The public 86-fixture Bash 5.3 gate is necessary but not the complete shell
regression gate. On the licensed native host, the sibling proprietary harness
must also run `make test-bash-system ARM=<unique-name>`: all 493 VSC shell TPs
drive `bin/bash` while `/vsc/cushim` is absent and external commands resolve to
VSC/host providers. Require it for release candidates and changes affecting
shell semantics. It is intentionally not a Bashy Make target or public-CI job:
the VSC suite is licensed and the OSS Bashy repository must not depend on the
proprietary harness checkout.

**Dragon delivery gate.** A verified Bashy change is not complete when tests
pass or a commit is pushed. After the umbrella pin is bumped, rebuild on Dragon,
install the canonical binary with `make install` (default
`~/.local/bin/bashy`), and smoke-test the changed command surface through that
installed PATH binary. Record the installed path and smoke result in the
handoff. Do not substitute a repo-local binary for this final check.

**Release checklist.** Releases are milestone-based semver bundles, not tags on
every commit. Before proposing a tag: the authoritative GNU Bash 5.3 gate has
zero regressions; the 493-TP VSC Bash-only/system-utility gate is green;
applicable POSIX/compliance and focused tests are green;
submodule commits and pins are pushed and clean; Dragon has passed the
rebuild/install/smoke gate above (including `bashy models`/`bashy agents` when
the fleet changes); and changelog/release notes are ready. The steward proposes
the tag/release after these gates; do not create one merely because a change
landed.

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
that walked root-equivalent paths. Later contained runs found recursive `cp`
deadlocks on `test_cp_fifo` and the `--copy-contents` directory-permission race:
coreutils `40eb4b6` fixed both, and each exact public case passed separately
inside the capped `bashy-cert` VM. They are no longer quarantined, but must
still never run directly. A later process snapshot tentatively implicated
`test_cat::test_fifo_symlink`, but a bounded regression proved coreutils already
follows and opens the FIFO symlink, and the exact public case passed in
`bashy-cert`; that temporary skip is also retired. Attempt 5 then found
`test_dd::test_random_73k_test_lazy_fullblock` blocks forever because the test
opens its FIFO writer without a deadline after the current SUT rejects
unsupported `iflag=fullblock`; coreutils `c12313d` implemented full-block
reads with a bounded FIFO regression, and the exact public case passed in
`bashy-cert`, so that temporary skip is retired. Attempt 6 then found
`test_dd::test_seek_output_fifo` deadlocks because both the SUT output and test
producer open the FIFO write-only. Coreutils `55960c0` now consumes the output
offset through a readable FIFO endpoint, its bounded regressions pass, and the
exact public case passed in `bashy-cert`, so that temporary skip is retired.
Attempt 7 then found `test_dd::test_sync_delayed_reader` deadlocks because the
SUT rejects `conv=sync` before opening `if=fifo`, leaving the test producer
blocked in its write-only FIFO open. Coreutils `08a2a44` implements standard
sync/block/unblock padding and fixes audited `bs` precedence; all six pinned
public dd FIFO shapes have bounded regressions, and the exact observed case
passed in `bashy-cert`, so that temporary skip is retired.
`scripts/uutils-scoreboard.sh` is the only supported entry point: it always
uses a disposable, non-root OCI container with hard memory, PID, and wall-time
limits, no network, and no host-root/home mount. Its permanent known-case
quarantine has no override. A killed, truncated, or denominator-inconsistent
cargo transcript emits no scoreboard. Run only `make test-uutils-safety` for
bounded harness validation. See `docs/uutils-scoreboard.md` and
`../docs/conformance-test-landmines.md`.

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
`ci.dag.md` is the shared CI graph for dhnt Go projects — other repos pull it in
by pinned reference (`include: gh:qiangli/bashy@vX.Y.Z/ci.dag.md`) and override
only the vars that differ, so a change here is cross-repo.
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

The authoritative release result is `make test-bash-container`: it bakes the
testee, this runner, and the pinned fixture corpus into one Linux image, then
runs non-root with an isolated tmpfs, no network, a read-only root, and a PTY.
That prevents host `/tmp`, locale, permission, and controlling-terminal state
from masquerading as shell regressions. Native `make test-bash` remains the
fast host-integration lane; it is useful diagnostics, but its result is not a
release verdict when the host cannot provide the fixture environment.

### Bash 5.3 fixtures (gitignored symlink)

`external/bash-5.3` is a **gitignored symlink** into a Bash 5.3 fixture tree.
`make test-bash`, `make test-bash-parallel`, `make test-bash-list`, and the
helper target create it automatically when absent: the released Bash 5.3
tarball is pinned by SHA-256, the required `tests/` and `support/` trees are
extracted atomically under the user cache directory, and the checkout symlink
points there. An existing local source-tree symlink remains valid. To provide
one explicitly instead:

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
- `plan-bashy-release-t0.md` — **`bashy release`**: the distribution verb (what bytes leave this machine, under what name — the one thing no orchestration verb owns). T0 = the local-first half in-process over `coreutils/pkg/release`: `bashy release --snapshot` builds → archives → checksums a `.goreleaser.yaml` subset and emits a `bashy-release-v1` ledger, with no network, no credentials and no tag. The whole GoReleaser CLI is NOT imported (measured: +77.3 MB, 277 new modules, 6 new MPL-2.0 deps); the tail (sign/sbom/publish/packages) stays binmgr-managed externals and is refused **by name** when a config declares it, never silently skipped. Records why the atlas group is `toolchains`, and why a snapshot's version is stated (`--version`) rather than guessed from a tag.
- `agent-bands-and-nicknames.md` — the shipped **band** (L1–L4 capability peg, normalized across providers — a vendor's own tier ladder is never mapped positionally) + **nickname** system on `bashy agents`/`models`. Bands live on the model and are inherited by the agent; `--min-band N` selects a roster (`bashy meet start --min-band 3` seats its own table and reports who it skipped). Canonical model names are version-explicit (`opus5`) and the family name (`opus`) is a *derived* alias that re-points itself on release — so a record never rots. Nicknames are assigned deterministically from the binding (same agent, same name, every host). Rules: speak the alias, record the address; a binding is canonicalized however it was spelled; a derived name never shadows a declared one. Read before any fleet-registry / agent-selection / routing work.
- **`bashy craft` + `bashy define`** — the living skill graph and the
  what-is-this-word resolver, both in `coreutils/pkg/{craft,lexicon}`. `craft` is
  the layer OVER the skills catalog: `find` asks for a capability in plain words
  (matching on what a skill GUARANTEES, so a query resolves a skill whose prose
  never used those words), `compose` renders it on demand at a **band** (0 = a
  runnable script needing no model, 4 = pure intent), and `learn`/`facts`/`fold`
  accumulate what running things taught. `define` answers "what is this word on
  THIS system" across verbs, agent bindings, skills, env vars, local commands,
  path segments, interfaces and mounts — reporting a command's resolved path and
  an alias's expansion, and classifying a credential WITHOUT echoing it.
  bashy contributes `internal/agentos/learn.go`, the ExecHandler middleware that
  records what each successful invocation taught. Two rules that bite: `define`
  must never gain a subcommand (its argument is an arbitrary user token, so a
  subcommand steals that word — `bashy define study` once made "what is the word
  study" unaskable), and `lexicon emit` must never render a `Location` (paths
  carry the operator's home dir, and emit writes into committed files). Both
  ratcheted. Design of record: `../docs/skill-graph-design.md`.
- `command-atlas.md` — the Command Atlas: the multi-axis agent-facing catalog of the whole command surface (classical group + execution tier + capability + idiom axes). Tables live in `coreutils/pkg/atlas` (coverage-test-ratcheted against `tool.Names()`); the bashy merge layer is `internal/agentos/atlas.go`; views via `bashy commands --view tier|group|capabilities`, `--tier/--group/--cap` filters, `--idioms`, `--atlas` (`bashy-atlas-v1`). Adding a verb/tool = add its atlas entry (the tests name what you forgot).
- `space-time-advisor.md` — the shipped space-time advisor: non-intrusive error-time hints (cwd/network/compute/disk + doomed-loop + network-fingerprint host memory) that steer agentic tools off doomed retries. Self-contained feature doc (dimensions, env vars, `bashy-advice-v1` JSON schema, scope/non-goals).
- `one-agent-control.md` — **the one control surface** every command that drives an agent CLI now steers through (`invoke` · `weave` · `meet` · `foreman`). `chat.Session` (Start/Say/WaitIdle/Turn) is the primitive — *Invoke is a question, Session is a conversation* — and it lives in `chat` because that is where `agentChildEnv` (secret scrub · single granted API key · shell-forcing · principal identity) lives. `agentpty` owns the wire (`TextFrame` = a sentence typed; `VerbatimFrame` = a keystroke), collapsing three divergent copies of one protocol. Why `meet --steerable` is a flag and not a default (a live turn under a THIRD-PARTY CLI has no boundary — it ends on silence, so it pays a quiet period out and a TUI startup in). **A tool that declares `events_arg:` escapes that**: it reports `turn.end` and bashy believes it, because that is a fact the agent asserted rather than a silence bashy interpreted — today only `ycode` does (see `first-party-harness.md`). Also: `foreman interrupt` (ESC as a real keystroke) — a queued message never reaches an agent stuck in a tool loop, because it reads its queue only between turns and that turn is never going to end. Read before any steering / `say` / `tell` / agent-launch work.
- `unified-inbox.md` — **`bashy inbox` is the one receive-side view** over MB, Meet boards, Bus notifications, and stable role addresses. It adds no store, preserves per-source cursors, watches all sources, and injects one budgeted block only at verified Bashy-owned turn boundaries; externally-started sessions remain explicit pull-only.
- `chat-interactive-launcher.md` — **`bashy chat` as the governed front door** for launching a third-party agent CLI *interactively*: the tool's NATIVE UX (agentpty's raw-mode local-TTY passthrough, not a bashy REPL) but with the fleet-selected model, full `agentChildEnv` governance, and a live-sessions registry (`~/.bashy/sessions/`) that makes the launched agent ADDRESSABLE — `chat sessions`/`steer`/`interrupt`/`attach`, later coach/meet. Selection: `--agent NICK` (specific) or `--band N`/`--tool T` (any operable one, reusing `SeatByBand`). `invoke` stays the one-shot (*Invoke is a question, Session is a conversation* — finally implemented). ycode is special-cased (already bashy-native → just launches it with the resolved `--model`). Companion to `one-agent-control.md`. Read before any interactive-launch / session-registry / chat-mode work.
- `unified-agent-assignment-visibility.md` — **`bashy agents` is the canonical live-work view.** Every managed launch, including short one-shot invoke work, publishes room membership while live; the roster reconciles room, weave, and sprint state without duplicates and exposes named/ad-hoc attribution to humans and JSON consumers. Read before changing any agent launch or assignment surface.
- `absence-of-evidence.md` — **the day's real product, and the codebase's characteristic failure.** SEVEN instances in one day of ONE shape: *a success state reached by the absence of evidence.* Declared fields nothing writes (`ConversationMessage.Usage`, `ExemptFromMasking`, `StreamOptions`, `SessionTotalCost`, 3 config fields), caps that bind and exit 0, a pricing fallback that bills an unknown model at Claude's rate. Every one produced a PLAUSIBLE ANSWER THAT WAS NOT TRUE, and four of them nearly got recorded as facts about a MODEL. Also: the four times my own instruments lied (`cmd | head && echo OK` chains off head's exit; `rm` on a receiver's open file; a bad `pgrep` pattern; an OTLP receiver silently dropping span events). Read before trusting any green check.
- `agentic-history-and-space-graph.md` — **the shipped agentic replacement for the `history` builtin, and the entity graph learned from it.** Two planes from one observation at the ExecHandler seam: TIME (`pkg/execlog`, every dispatched command, ordered, prunable) and SPACE (`pkg/spacegraph`, hosts/endpoints/accounts and the relations between them, bi-temporal, `0600`, **no export path — every node is identity**). `graph learn` pipes what the corpus supports into kb as **candidate** pages carrying an ADDRESS into the stream, not a copy; `graph evidence` walks it back, and reports honestly when the records have been pruned (the claim outlives its evidence). Load-bearing rules: time is never in a key (put a clock in one and the store silently fills with n=1 singletons); FAILURE TEACHES NOTHING (a transport failure is unattributed — correction is by supersession on positive evidence); every read verb prints its coverage. **Read `../docs/knowledge-substrate-reconciliation.md` first** — it demotes these two from "stores" to a stream and a view, with kb as the one truth.
- `observability.md` — the shipped OTel plane. bashy could RUN a collector (`bashy otel`) and fed it NOTHING — it was the one tier of the whole stack missing from the umbrella's `service.name` set. Two primitives, chosen from what six hours of debugging could not see: **Provenance** (a value next to WHERE IT CAME FROM — the only bug caught by a signal was caught by `from_provider=false`) and **BoundHit** (a limit records when it BINDS — especially when the run recovers). Plus a span per command at the ExecHandler chokepoint, including the EXIT CODE. Stack trimmed 286 MB → 109 MB (−61%) by going Victoria-only: jaeger (2,240 deps) → VictoriaTraces, perses (1,478) → vmui, collector (833) → three proxy map entries, prometheus (556) → VictoriaMetrics. Pure standard OTEL env vars; unset endpoint is a total no-op; `cmd/bash` links none of it.
- `audit-log.md` — the shipped compliance audit trail: a tamper-evident, hash-chained, secret-redacted record of every dispatched command with agent attribution and Command-Atlas effects (`bashy-audit-v1`; NIST AU-3/AU-9). Opt-in via `BASHY_AUDIT`, off by default, never in `cmd/bash` / `--posix`. Read side is `bashy audit {status,tail,verify,export,path}`; core is `coreutils/pkg/policy/audit`, the ExecHandler middleware is `internal/agentos/audit.go`. Records; does not block (policy engine) or contain (OS sandbox) — the un-bypassable record of the agentic+interactive command path, composes with auditd/EDR. Deferred: OTel export, signed checkpoints, gitleaks-grade redactor.
- `fips-140.md` — the shipped FIPS 140-3 build mode: `make build-fips` (`GOFIPS140=v1.0.0`) builds both binaries against the Go Cryptographic Module (CMVP #5247); pure-Go, no cgo/BoringCrypto. Use `GODEBUG=fips140=on` (the build-fips default — keeps `md5sum` working), NOT `fips140=only` (rejects MD5) for a general shell. State surfaced in `bashy doctor` and `bashy context --json` (`runtime.fips140`). A FIPS-built `bin/bash` still passes 86/86. Pairs with the audit log for the FedRAMP/CMMC procurement story.
- `plan-bashy-ask-human-input.md` — **`bashy ask`**: get an ad-hoc value from the
  HUMAN from inside an agent session, over a channel the agent does not own
  (controlling terminal → GUI askpass → out-of-band rendezvous), returning a PATH
  rather than the value. Exists because a command run by an agentic CLI does not
  own its stdin or stdout — measured: Claude Code `setsid`s its children, so
  `/dev/tty` is ENXIO and the obvious implementation cannot work. Replaces the
  `/tmp/x` habit. Engines are `coreutils/pkg/{ctty,ask}`; bashy contributes the
  four registration points. Design of record: `../docs/bashy-ask-human-input-design.md`.
- `bash.md`, `agentic-extensions.md` — background references, not active plans.

POSIX-conformance frontier (the active layer now that bash-5.3 is 86/86 — driven via `suites.md` + `dag.md`):

- `plan-posix-conformance.md` — plan of record for the POSIX-mode conformance push (the differential suites + yash scoreboard).
- `conformance-statement.md` — the standing conformance claim; `shell-conformance-comparison.md` / `cross-shell-conformance-baseline.md` — bashy vs other shells.
- `posix-mode-behaviors.md` — catalogued `--posix` behaviors; `builtin-vs-external-conformance.md` — builtin/external divergence notes.
- `posix-cert-handoff-runbook.md`, `posix-cert-preflight-status.md`, `fidelity-ceiling-assessment.md` — VSC-PCTS certification runbook + status + the hard-ceiling assessment.
- `yash-conformance-gap.md` — the yash-scoreboard failure analysis behind the headline number in `docs/TODO.md`.
- `zsh-scoreboard.md` — the zsh Tier-0 own-suite baseline (`make test-zsh`, `tools/ztst` runner); INFO metric, not a gate.
- `chunked-fleet-conformance-plan.md` — the chunked/fleet/container conformance lanes in `dag.md` (`test-bash-chunks*`, `yash-chunks*`): chunk count is a corpus property pinned in a committed manifest, and the authoritative run stays single-host + unchunked (`make test-bash-container` runs all 86 serially in one hermetic image) — campaign mode never speaks for it.
- `isolated-test-lanes.md` — the multi-agent container/runbook: weave workspace → lane → independent OCI container/result namespace; public self/Bash53/Yash commands, private POSIX A/B/C/D lanes, repeated-profile concurrency, swappable base-image adapters, capacity, status, and cleanup.
- `ci-failure-autorepair-plan.md` + `config/ci-failure-fixer.env` + `scripts/ci-failure-{router,fixer,gate}.sh` — the `.github/workflows/ci-failure-report.yml` lane that routes a CI failure to a **fixer** run (the band-selected agent that repairs one failing gate — a lighter role than the SDLC `conductor`, which is the escalation target for a fix that needs orchestration).
- `bashy-v1.0.0-readiness.md` — the release-readiness ledger.
- `agent-adoption/matrix.md` — which agentic CLIs are verified running on bashy as their shell (the `force-agent-shell` skill's evidence base).
- `first-party-harness.md` — **why ycode is in the fleet, and what it actually buys.** All four "still owed" items shipped 2026-07-14. The differentiator is NOT that it wins a bake-off (it lost — slowest, most code): it is the **event channel**. `--events` emits `turn.start`/`tool.call`/`turn.end` as NDJSON on both the one-shot and TUI paths, so a turn's end is a FACT THE AGENT REPORTS rather than a silence bashy interprets (`WaitIdle`, 25s). `turn.end.text` equals `--print` stdout exactly. Not yet reached: server mode (the agent loop lives in the server process, which never sees the client's `--events`). Read before any harness-selection or `chat.Session` work.
- `plan-agent-harness-positioning.md` — gap analysis and phased plan for positioning
  bashy as a **harness kit**, not a privileged Go-coded harness: governed substrate,
  conductor, authoring kit, and bidirectional peer. Records the plain-bash worked-loop
  gate, the later Bash++ port, and why front-door verbs need the same P0 governance seam
  as shell-resolved commands.
- `plan-bashy-llm.md` — P0.5 design of record for the missing stateless model-call
  primitive: one JSON request/response, model resolution through the fleet catalog,
  Ollama + OpenAI-compatible T0 providers, no tool execution, and record/replay. Read
  with the positioning plan before any native model-call or harness-authoring work.
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
- `skills/steward/` — the steward role: the host's authority record, the
  handover contract, and the tick loop.
- `skills/check-messages/` — read the fleet message board (`bashy mb`) at the
  START of a turn, before planning, so a second agent doesn't redo or contradict
  work already taken. Requires `has=bashy`.

`skills/embed.go` and `bashy skills list` are the sources of truth for the set
(seven today); this prose drifts, they don't.
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
- **Pure Go only** for the core: no CGo, no C libraries. Two `cc` invocations
  exist and neither is CGo: `test-bash-helpers` builds Bash's own test helpers,
  and the unix build compiles `native/siglaunch.c.in` as a standalone launcher
  process (see §The unix binaries are a C launcher over the Go binary). Both are
  our own code; cross-compiled release artifacts are `CGO_ENABLED=0` pure Go.
- **Runtime download + exec ≠ bundling**: tools bashy downloads and runs as
  separate processes (podman/ollama/gh/loom/act/…, and fetched test suites) are
  not bundled — separate programs on their own licenses, no propagation. Prefer
  permissive anyway.
- **Required + no permissive substitute → build from permissive source** via the
  self-provisioning toolchain (`bashy go`/`cmake`/`clang`), in CI or on demand —
  never ship a non-permissive prebuilt.
