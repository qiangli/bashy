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

File-by-file map: **`docs/source-layout.md`**. Orientation: `cmd/bash` is the
pure drop-in entry (no AgentOS imports), `cmd/bashy` wires `internal/agentos`
into the shared `internal/cli` core; `internal/agentos/` is the AgentOS wiring
over `../yoke` + `../coreutils` (ExecHandler ring, front-door dispatch, learn
middleware, contracts natives); `tools/bash53suite` is the ONE fixture runner;
`skills/` is embedded; `native/` is the unix C launcher; `scripts/` holds the
supply-chain and build-isolation lanes `make test` runs first.

## Name

**Three substrates, three names.** Bashy is built in three cumulative layers, and
each has exactly one short name. Use these words; the corpus previously carried
four competing framings for the same triple.

| # | Short name | What it covers | Carried by |
|---|---|---|---|
| 1 | **Classic** | GNU Bash 5.3 compatibility + POSIX 1003.1-2016 conformance, **plus the pure-Go coreutils userland** | `bin/bash` (shell) + the coreutils ExecHandler |
| 2 | **Bash++** | the opt-in language extension: Go-shaped constructs, plus the Python/TypeScript ergonomics Go omits (**Bash#** is a tier *inside* it, never a third dialect) | the `sh` parser/interpreter dialect seam |
| 3 | **Yoke** | the agentic framework — tools, policy, durable sessions, communication, coordination, workflow, observability | `bin/bashy` (`internal/agentos`, planned `coreutils/pkg/yoke`) |

Spoken as a ladder: **Classic → Bash++ → Yoke.** The ordering is load-bearing —
a higher layer may never weaken a lower one.

**Naming rules:**

1. **Classic** names *bashy's* layer 1. **GNU Bash** or **stock bash** names the
   external reference implementation. Never bare "Bash" for either — that
   ambiguity is why this vocabulary exists. Not "basic": this is the most
   rigorous layer in the repo, not a cut-down one.
2. **Bash++** in prose (capital B, `++`). Never `bashy++`. Machine tokens are
   separate and already settled: `--bashpp` canonical, `--bash++` human alias,
   `syntax.LangBashPP` / `"bashpp"`.
3. **Yoke**, capitalized, no prefix — never "bash yoke". Layer 3 does not exist
   in the `bash` binary at all (`cmd/bash` structurally cannot link
   `internal/agentos`), and Yoke is a general agent framework shared with ycode,
   not a bash feature.
4. Do **not** prefix all three with "bash". The prefix is not parallel: layer 1
   *is* Bash, layer 2 is a superset *of* Bash, layer 3 is *not Bash at all*.
5. `cert` still names the *workstream*; `compat` and `conformance` still name the
   two *promises* inside Classic. Those are a different axis and are not renamed.
   Retired as layer-3 names: "agentic extension", "agentic superset", "O3"
   (`O3` keeps its own meaning as the ollama/oci/otel tool bundle).

**Pillars are not substrates.** `docs/philosophy.md` §4 decomposes the thesis into
three *pillars* — compatibility → capability → agency — and that is a different
axis from the three substrates above. A pillar is what bashy must have to be
trustworthy (none is removable); a substrate is a surface you address by name. So
**Bash++ is not a pillar precisely because it is opt-in** (`--bashpp`, off by
default), and **the userland is not a substrate** because it is not separately
addressable — it is *how Classic keeps its compatibility promise on three
operating systems*. Pillars I and II both land in Classic; pillar III is Yoke.
Don't cite "the three pillars" when you mean the three substrates.

**The backronym nests; it does not collide.** BASHY expands to *Bashy's Agentic
Shell Harness Yoke*, whose head noun is **Yoke**, modified by *Agentic Shell
Harness*, possessed by *Bashy* — so the phrase names **a yoke belonging to
Bashy**, which is layer 3. The acronym names the product; its expansion names the
product's top layer. Recursive in the GNU lineage, which is the joke. Design of
record for layer 3: `../docs/bashy-yoke-framework.md` (planning-only, deferred).

## Module wiring

`go.mod` requires the flat-sibling deps, resolved by `replace`:

```
replace mvdan.cc/sh/v3               => ../sh
replace github.com/bashsharp/bashsharp => ../bashsharp
replace github.com/qiangli/coreutils => ../coreutils
replace github.com/qiangli/yoke      => ../yoke
replace github.com/ergochat/readline => ../readline
replace github.com/filebrowser/filebrowser/v2 => ../filebrowser
```

`../sh` is the interpreter engine; `../bashsharp` is the **Bash# language's
front door** (Sprint 211: `sh` ← `bashsharp` ← `bashy`; Bash# was Bash++
until 2026-09-18 — rail5/bashpp owns that name, see
`bashsharp/docs/naming-collision.md`; the engine's `BashPP` identifiers are
deliberately unchanged) — `bashsharp/front` is the dialect selector + the
direct Go-source interface that `internal/cli` calls (`--bashsharp`,
`--source=go`, `--check`, `--go-list`; `--bashpp`/`BASHY_BASHPP`/`.bpp` are
deprecated aliases that warn once), `bashsharp/transpile` is
what `bashy transpile` dispatches to and the import that wires the Go front
end into `cmd/bashy` (the `bash` drop-in never imports it — the
`TestGoSourceFrontEndIsNotLinkedIntoClassicBash` ratchet; it DOES reach
`front`, as it has carried `--bashpp` since Sprint 97). The engine itself —
the evaluator, `lower`, `gosource`, `polyglot`, the grammar — still lives in
`sh` (`bashsharp/docs/seam.md` says why), so a Bash# *semantics* change is an
`sh` change and a Bash# *front* change is a `bashsharp` change; the contract
natives (`@require`/`@ensure`/`@guard`/`@trace`/`@retry`) and advice stay
here in `internal/agentos` because they need yoke policy + OTel, which
`bashsharp` may not import. `cmd/bashsharp` (in bashsharp) is the language's
own binary, what the `bashsharp-tests` corpus harness measures. `../coreutils` is the AgentOS hub that
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
rebuild/install/smoke gate above (including `bashy model`/`bashy agent` when
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

`tools/bash53suite` arms a procguard before launching each fixture. Abrupt
harness death kills the fixture process group, and normal completion or timeout
also removes background descendants left in that group. On Windows the same
contract is provided by **Job Objects** (`proc_windows.go`, Sprint 216): the
harness puts itself in a kill-on-close job so every fixture is born contained,
and each fixture gets a nested job carrying the memory cap that is terminated
on reap. The Windows tests prove it on every push; the suite itself is
MEASURED there by `conformance.yml`'s `bash53-windows` job (tags /
`workflow_dispatch`, `scripts/ci-bash53-windows.sh`), which publishes exact
runnable/pass/fail/timeout/skip counts as an artifact. Since Sprint 245 that
leg runs against **bashy's own userland** (the pure-Go `yoke` multicall laid
out as a POSIX root under `BASHY_ROOT`, with the corpus's recho/zecho/xcase
helpers served by the harness binary itself), and the latest measurement is
**64/86** (run 35720151852, 2026-09-22) — up from 23/86 on the Git-Bash
userland. **Never quote 86/86 for Windows**, and never quote any Windows
count from anything but that artifact (see `docs/plan-bash53-windows-leg.md`
and the umbrella's `docs/sprint-245-delivery-evidence.md`).

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
**Bodies run in the invoking cwd (make parity, Sprint 185)** — `-f` only picks
the file; there is no `-C`: `bashy awd DIR -- bashy dag …` is the one
directory mechanism (`awd` is a front-door verb as well as a builtin). A
` ```bashpp ` body runs as Bash++ and may declare a `~~~py as py … ~~~`,
`~~~ts as ts … ~~~`, `~~~rs as rs … ~~~`, `~~~c as c … ~~~`,
`~~~cxx as cxx … ~~~`, or `~~~go as go … ~~~` fence and call
`py.main()` / `ts.launch()` / `rs.launch()` / `c.launch()` /
`cxx.launch()` / `go.launch()`;
`~~~bash` and `~~~sh` use fresh in-process Bash-5.3/POSIX child interpreters
(positional string args, stdout result, non-zero status error; never host bash);
`examples/dag/{mini-swe-agent,nanochat}/dag.md` (Python, `make
smoke-dag-python`), `examples/dag/{opencode,openclaw,hermes-agent}/dag.md`
(TypeScript — Hermes has both fences in one body; `make smoke-dag-typescript`,
Sprint 186), `examples/dag/{uv,codex,bun}/dag.md` (Rust — `smoke` reads the
workspace from a fence, `run` launches the `cargo build` output from one;
`make smoke-dag-rust`, Sprint 188) and
`examples/dag/{ffmpeg,curl,git,tesseract,llama.cpp,cmake}/dag.md` (C and
C++ — `smoke` includes the checkout's own self-contained header at compile
time, `run` launches the `configure`+`make` / `cmake --build` / `bootstrap`
output through `popen`; `make smoke-dag-c`, Sprint 190), and
`examples/dag/{gh,hugo,caddy}/dag.md` (Go — imports each checkout's package,
then launches its `go build` output; `make smoke-dag-go`, Sprint 192) are the
worked examples, one real repo each — see `docs/dag.md` §Bash++ bodies. A
body may also hold a TEXT fence (Sprint 234, B30): `~~~dockerfile as img` /
`~~~tf as iac` / `~~~k8s` / `~~~helm` / `~~~skill` / `~~~dag`, whose alias
exposes the processor's declared verbs with their effect atoms (`@guard`
denies an excess verb with 126), or `~~~<type> as <alias> !<runner>` for a
runner that declares its own `methods`; `examples/dag/caddy/dag.md` `image`
and `examples/quickstart/pipeline.bsh` under `make smoke-dag-text`. The
MANIFEST fences (Sprint 238) — `~~~cargo`, `~~~pyproject`, `~~~gomod`,
`~~~cmake`, `~~~makefile`, `~~~package` — carry a project manifest inline and
drive the toolchain bashy provisions in the directory `awd` chose, the tree
byte-identical (`examples/manifests/`, `make smoke-dag-manifests`); a
`~~~gomod` fence is the `~~~go` code fence's module. A dag body
must not lean on bashy's `sed`/`grep`/`cut`/`sort`/`tr` for a check: they
refuse the macOS default `LANG=en_US.UTF-8` (coreutils' ctype/collate locale
gate) — the examples cross-check with shell builtins only. Two more
body-vs-terminal differences (Sprint 190): `make` in a body is bashy's
in-process POSIX make, so a GNU `Makefile` is driven with `env make …`; and
the body sees PATH only — the front-door shims (`bashy cmake`) are not
applied inside it, so a body's `cmake` must be on PATH. The TypeScript runtime is chosen per repo in the target's
`Env:` (`BASHPP_TYPESCRIPT_RUNTIME=bun` where the repo's own imports need Bun).
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

Push CI is unit and mock tests only (decided 2026-09-17). The GNU bash-5.3
conformance suite (the former `bash53-gate` job) runs from
`.github/workflows/conformance.yml` on `v*` tags and `workflow_dispatch`, and
the licensed POSIX shell arm never runs in Actions at all — both are release
gates (`kb:release-bashy`), not push gates.

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

Read `docs/TODO.md` to know what is open — it is the scoreboard and the todo
list. A todo needs no sprint to exist; sprint work is tracked as stories on
the card, and the sprint card (spec-ref, acceptance, continuity) carries the
request, plan and details. Delivery commits
carry `Sprint:` / `Story:` / `Story-ID:` trailers. After a change:
`go test ./...` and `make test-bash`, then commit.

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

`docs/` holds the planning + status corpus; the index is **`docs/INDEX.md`**
(one entry per load-bearing doc — add yours there, not here). Where to start:
`docs/contracts.md`, `docs/command-atlas.md`, `suites.md` (root),
`docs/licensing-supply-chain-policy.md`, `docs/TODO.md` (the scoreboard).

## Skills

`skills/` holds the tier-2 **workspace** agentic skills bashy ships (the
userland is tier 1, clusters tier 3). They are **compiled into the `bashy`
binary** via the `//go:embed` directive in `skills/embed.go` (surfaced by
`bashy skill`), so adding a skill means dropping its directory here AND adding
it to that directive. Each is a self-contained Anthropic skill
(`SKILL.md` actionable checklist + optional `reference.md` deep companion),
brand-neutral and driven by bashy's own tools:

- `skills/bashy/` — how to drive bashy itself as an agent (start with
  `bashy inspect context --json`; dry-run/check/run envelopes; code-intel verbs).
- `skills/conductor/` — drive a fleet of agent CLIs to a verified goal over
  `bashy sprint` + `bashy weave` (decompose → isolate → gate → converge, loop
  until a verifier passes); TDD-at-fleet-scale is the canonical mode.
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
- `skills/inbox/` — read the fleet message board (`bashy mb`) at the
  START of a turn, before planning, so a second agent doesn't redo or contradict
  work already taken. Requires `has=bashy`.
- `skills/sprint/` — the sprint seat: card, goals, stories, checkpoint,
  handoff.

`skills/embed.go` and `bashy skill list` are the sources of truth for the set
(seven today); this prose drifts, they don't.

**Record vs canonical.** `SKILL.md` is the on-disk canonical form of every
skill — the de facto standard third-party harnesses read — and `bashy skill
show <name>` prints it byte-identical. `bashy skill show --yaml|--json`,
`add <file.yaml>|-`, and `export --yaml` project the same folder to and from
a `kind: skill` RECORD (a lossless bundle: frontmatter fields + files verbatim,
identity derived from the `skill.dhnt` canonical line, never from the YAML).
The record is for the catalog and the wire, never the authoring form. Edit a
skill with `skill set`/`edit` (copy-on-write into the local ring; embedded
skills are immutable), never by hand under `~/.config/bashy`. Design of
record: the umbrella's `docs/bashy-action-model.md`. Ecosystem-specific and internal
operational skills, including `go-repo-health`, live in the umbrella's
`skills/` overlay and are not compiled into the public binary.
- `skills/force-agent-shell/` — attested check that agentic CLIs route their
  shell commands through bashy (so the pure-Go userland, the advisor, and OTel
  apply to everything an agent runs). Run as a convergence gate before an
  unattended fleet run: `bashy skill run force-agent-shell` (exit 0 iff the
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
