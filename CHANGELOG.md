# Changelog

All notable changes to bashy are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows
[docs/release-roadmap-and-versioning.md](docs/release-roadmap-and-versioning.md).
Full per-release notes, where they exist, are in [docs/releases/](docs/releases/).

bashy is pre-1.0. Classic Bash remains the default language mode in every
release; Bash# and Yoke are opt-in layers on top.

## 1.0.0 scope

v1.0.0 ships three pillars on one binary, plus the services and release
engineering under them:

- **bash** — a GNU Bash 5.3-compatible shell and the declared POSIX
  1003.1 Shell and Utilities profile with its documented limitations.
- **Bash#** — the opt-in language layer: Go-shaped constructs in script text
  and fenced blocks of Python, TypeScript, Rust, C/C++, C#/PowerShell, Go and
  bash/sh.
- **Yoke** — the agentic userland, as an MVP that is a complete foundation:
  registry, model endpoint, tool protocol (bashy as an MCP server), reference
  agent, knowledge and skills, work tracking, communication and a safety floor.

Embedded languages, the agentic surface, MCP and `kb`/`graph`/`skill`/`craft`
were once planned for v1.1 and v1.2; they are part of 1.0 and those deferrals
are retired. What remains after 1.0 is depth, not presence. A pre-1.0 entry
below is a milestone on the way, not a claim that the 1.0 gates are met:
conformance, signing and install-channel verification are re-measured on the
frozen release candidate before the tag.

## [Unreleased]

Draft notes for 1.0.0-rc.39: everything user-visible merged since
`v1.0.0-rc.38-dev`, by theme. No changelog entries were cut for
rc.28–rc.38; the sprint-coordination verbs that landed during the rc.38
cycle are recorded here so the gap is not silent. Docs-only and test-only
commits are omitted except where they change a public claim.

### Added

- **Sprint coordination for agent fleets** — `bashy sprint await` blocks on
  typed submit events instead of polling; `sprint directive add|list|rm`
  keeps standing operator rules that are injected into worker briefs;
  `sprint reject` records a rejection across story and run. Messages can be
  addressed to the `conductor:<sprint>` seat instead of an agent name, the
  seat holder is stamped on the mail, and sender claims must match the
  sending tool and model. The manager playbook in `bashy sprint --help`
  and the conductor skill now describe this loop exactly as it runs.
- **Waiting on your inbox** — `bashy inbox --wait` accepts
  `--until-count N` and `--match REGEX` (a regular expression over body,
  sender, recipient, topic, room and source), returns as soon as the condition
  holds, and exits 3 on timeout, so a script can block on coordination the
  way it blocks on a process.
- **Declared session commands** — `bashy chat command` runs the named
  commands a live agent session declares for itself (its TUI's own verbs)
  instead of typing blind keystrokes, with exit codes that say whether the
  command was delivered. Steering a session through `bashy chat steer`
  remains the free-text path.
- **A bridge from `dag.md` to make** — `bashy dag --makefile` generates a
  Makefile that forwards every target to `bashy dag`, headed by a
  do-not-edit banner that teaches the reader what to edit instead
  (`dag.md`) and how to regenerate. The make side carries no task graph of
  its own, so the two can never disagree about what a target does.
- **Consumer-shapeable example apps** — the desk viewer and chart and the
  tutor session display are now shaped apps end to end: registered with
  `bashy app add --type shaped`, served on the console-assigned address,
  Genie injected through the console mount, and each ships concrete
  example wishes (`genie.examples`), as do form, notepad, board,
  whiteboard and table. A member-principal end-to-end script drives the
  wish → verify → preview → approve → undo lifecycle across all of them,
  and the KB runbook "Enable an app for consumer shaping"
  (`bashy kb search "enable an app for shaping"`) distills the steps for
  any app owner.
- **Local runtime discovery** — `bashy app show NAME --runtime` reports
  the live loopback address, data directory and console URL of a running
  shaped app from the record the console publishes when the app becomes
  ready. App CLIs (viewer, chart, session) resolve through it when no
  explicit address handoff is given, and fail with one actionable error —
  open the app, or pass the handoff — never by falling back to a fixed
  development port or a second private server.
- **Podman machine autostart (outpost)** — opt in with
  `settings.autostart_machine` and outpost starts an existing podman
  machine again after a reboot, so a restarted host recovers its
  containers without a login. It is off by default, starts only a machine
  that already exists, and backs off instead of spinning when a start
  fails.

### Changed

- **Platforms and images** — macOS Intel (darwin/amd64) is no longer
  supported; macOS releases support Apple Silicon (arm64). The public
  GHCR scratch image is no longer published: install Bashy, then use
  `bashy self image` to build the local FROM-scratch image from
  checksum-verified scratch binary assets.
- **`make` without a Makefile** — inside bashy, `make` in a directory with
  no makefile but a `dag.md` now falls through to `bashy dag`, and
  `type make` tells the truth about which implementation will run.
  Bashy's own remaining Makefile-only targets were ported to `dag.md`
  unchanged in what they measure, and the repo's scripts and CI call
  `bashy dag` directly.
- **Unavailable means unavailable** — a capability whose runtime is absent
  (for example, no podman socket on the host) now rests in a visible
  `unavailable: <reason>` state: health is not ok, the reason shows in
  status, and no restart ladder spins against a machine that simply lacks
  the runtime. It recovers on the next successful activation. Previously
  such a capability could report active and healthy while doing nothing.
  Outpost's podman capability reports the same way: socket absent, machine
  starting and start backoff are all `unavailable: <reason>`, never a
  false healthy state, with no restart churn while it waits.
- **`bashy sprint show` is fast** — rendering a sprint card no longer
  scans unrelated stores; on a large board it completes in seconds, and
  goal stories living in submodule checkouts resolve like local ones.

### Fixed

- **The git applet speaks worker** — the in-process `git` now serves the
  forms scripts and agents actually use: `commit -q`, `--allow-empty` and
  `-c user.name/-c user.email` identities, `-q`/`--quiet` on checkout,
  push, pull and fetch, and `checkout --detach` for revisions. Forms it
  cannot honor fail loudly; flags are never silently dropped.
- **Steering a Codex session submits** — `bashy chat steer` to a Codex
  TUI could type the text and stop at the input box; the message is now
  submitted, and an explicit Enter cannot fire against a session whose
  gate is not live.
- **Headless Chrome stays out of your keychain** — ephemeral Chrome
  launches (browser probes, desk PDF/export) run against isolated
  profiles that never touch the OS keychain or your real browser profile,
  so automated checks no longer interrupt you with system prompts.
- **Coordination correctness** — `bashy weave pull` exits non-zero and
  explains on stderr when nothing was merged instead of reporting
  success; revising your own room card is an update, not a second join
  event; `sprint review`, `grade` and `merge` grew their own `--help`
  one-liners instead of borrowing another verb's; sprint and foreman
  test runs are isolated from the host's real boards and queues.

References (story ids, by theme): fleet coordination 48a92d652802,
561bac1b3623, f8fed0a7bd83, 6b7b112d4c61, 188896bb1660, 1391d5686ba1,
91f04d61150d, 3347a7019443; inbox waiting 1ee0c4e59988; session commands
and steer 6254c3c64248, c00eb6a73a34; dag/make fa9fcc8de90d,
d9bf47e9554b, a97acb748321; consumer shaping 31d1e09e350e, 22debb3fbad2,
c97cad3dc8af, 98119d6d55a3, 8e5204e127c4; unavailable state and podman
autostart 9deb591a820c, ab6b9b872ac1;
git applet 4c60ebe893cd, 39669b0dd32d; headless Chrome e1b0b5ae8d15;
help texts 900e67575761.

## [1.0.0-rc.27] — 2026-11-07 (promoted)

Cut from the unreleased pool that accumulated since [0.32.0]; the earlier
rc-series candidates were never cut here. The changes new since
`v1.0.0-rc.24-dev` are summarized first, by theme — docs-only commits are
omitted except public-claim corrections — and the earlier unreleased entries
follow. Release notes: [v1.0.0-rc.27](docs/releases/v1.0.0-rc.27.md); open
issues: [known-issues ledger](docs/releases/v1.0.0-rc.27-known-issues.md).

### Added — since v1.0.0-rc.24-dev

- **Software shaping** — `bashy shape` is the CLI front door to the shaping
  lifecycle: `wish` (isolated candidate generation, or `--from` a workspace),
  `show`/`diff`, `verify`, `preview`, `approve` through the sealed activation
  gate, `undo`/`restore`, and `submit`, which opens a reviewable source
  proposal and an ordinary weave through the existing gates. CLI- and
  service-kind apps shape with build/test argument vectors held outside
  candidate source; native versions activate through immutable side slots
  while the installed release and PATH stay intact; bashy itself is
  shapeable, with status, undo, restore and the authoritative control paths
  always executing the retained released implementation. The same lifecycle
  reaches the paired console's remote API, and the shared shaping engine
  works in a standalone bashy.
- **Desk suite** — a shared desk kit with one Markdown renderer and one
  family look for every tool: notepad with multi-format export; forms with
  persistent questionnaire rounds, replyforms from browser extracts, and
  many respondents with a poll preset; charts with offline SVG widgets and
  OTLP input; and a view-only viewer for books of agent reports.
- **Tutor** — classroom sessions on the shared desk kit: `plan`/`apply` with
  an editable class format and agent draft, `serve`/`join`/`stop` with
  projector, teacher and per-student pages, guardrailed per-student nudges,
  session prep before class, owner-controlled tempo (pause/resume, time,
  goto, skip, move), one-command demo and go flows as dag targets, and
  readable participant links.
- **Klatch** — `serve` runs on the owner's own machine (signed pin, verified
  bundle) and reaches peers over the outpost mesh with `--mesh`; `invite`
  issues one-time sign-up links straight into the vault; generic post/topic
  verbs run as a bot. End-to-end checks cover the invite/chat happy path
  through the satellite in the owner's browser and the runner's
  pass/fail/BLOCKED decisions.
- **Repair profiles** — every conformance suite runs through the
  bashy/Bash# runtime with bashy's own POSIX utilities on the suite PATH,
  `-list` fails closed on an empty `-only` selection, and the outpost
  profile tests the shipped `CGO_ENABLED=0` build.
- `claim who PATH` — the four-source path-owner join; lapsed claims are
  hidden by default and only the covering root is named.

### Fixed — since v1.0.0-rc.24-dev

- CLI: an empty `sh -c` command operand is recognized, positional flags
  after `-c command` are preserved, and direct command-string callers keep
  working; a registered agent keeps its mailbox when its name is ambiguous.
- Process supervision: a Linux process counts as live until every thread
  exits, captured orphan-group members are joined before reclaim, unknown
  process liveness is never read as an exit, and malformed proc identity
  fields are rejected.
- Service telemetry is typed and bounded, with dispatch context preserved.
- Darwin descriptors are preserved across registered commands and startup,
  with producer-isolated SIGPIPE handling.
- The reviewed Bash# fence runtime and fixture controls are integrated; an
  absent fence method is an explicit refusal.
- Shell runtime repairs consumed through pins: virtual-subshell self-signal,
  command-substitution EXIT trap, strict POSIX getopts and asynchronous EXIT,
  bounded unset scanning, indexed arithmetic, alias grammar, Sh pointer
  metadata race, SQLite object write transactions, and Bonsai log rotation.
- The managed Linux podman engine stages its private nft helper; the tutor
  demo cleanup kills the server process tree (exit 0 on Ctrl-C) and tutor
  server builds are isolated from umbrella Go workspaces.

### Build and supply chain — since v1.0.0-rc.24-dev

- The tree moved onto standalone module pins: source metadata regenerated
  from the pins, workspace sources distinguished from published pins, linked
  Yoke and Coreutils provenance synchronized, and ycode example agents
  resolved from pinned modules instead of the umbrella layout.
- `sh` is pinned to public master with the merged fence runtime; yoke is
  pinned to main; the Klatch fleet pin carries v1.2.6 (outpost single
  sign-on, team mode, SQLite busy timeout).
- The local-model Ollama engine's CUDA runner ships per-tool prereleases
  with `libggml-cuda.so` (`$ORIGIN` RUNPATH, SONAME closure audited, NVIDIA
  runtime libraries host-required), is admitted by a checklist record
  instead of an unconditional hold, and builds the runner on go1.27.3
  (GO-2026-6809).
- CI: native validation stages are bounded separately, conformance testees
  build portably, the repair root derives from its workspace, and Windows
  repair suites run contained.
- Shrink-only import ratchets guard the `cmd/bash` and core/cert-base import
  graphs.

### Changes since v0.32.0 (earlier unreleased entries)

The entries below predate `v1.0.0-rc.24-dev` and were never cut into a
release section; they ride along in this candidate.

#### Added

- `bashy explain go` serves the Go delta table (text, `--json`, `--tsv`, and a
  refusal lookup).
- `bashy mod` front door over the shared module-pin library; `bashy doctor`
  reads sibling pin checks from `go.mod` pins, including inside a `go.work`.
- Release SBOM: an SPDX 2.3 generator, a permissive-license gate and a CI
  closure check.
- Linux packages in the release pipeline, and a Winget handoff.
- A local-model door end-to-end test: pull a model, `bashy llm up`, then chat
  through both the OpenAI-compatible and Anthropic-compatible paths.
- `bashy sprint hooks install`: fail-closed commit provenance guard (Sprint / Story /
  Story-ID trailers, Story-ID the full 12-hex id) plus a pre-push range check.
- A directory whose makefile is a `GNUmakefile` is handed to the host GNU
  make.
- `SECURITY.md` (how to report, supported versions, scope) and this
  `CHANGELOG.md`.

#### Changed

- The Darwin `bashy` release now builds with `CGO_ENABLED=0`, like Linux and
  Windows. The product runtime build exposes the Go runtime's snapshot of
  inherited ignored signals to pure Go, replacing the pre-Go C constructor.
  See [build evidence](docs/public-claims-evidence.md).
- Sibling modules are consumed as real `go.mod` pins rather than a separate
  sibling-pin file; outpost is a `go.mod` tool pin built by the release, never
  linked into bashy. A standalone clone builds without a yoke source tree.
- The shell runtime stamp is read from the sh fork version in `go.mod`.
- `install-agent` retains execution logging (installed shims emit execution
  records), and `out` graduates out of `experimental` into the default atlas.
- Release: configurations beyond the built-in subset are delegated to the
  provisioned engine.
- CI: the app service end-to-end test runs on every native runner; the
  Windows build, vet and test step has a 20-minute budget; source-routing
  tests run in CI.

#### Fixed

- The installer enforces the corrected shell-pair contract; `bash` and `sh`
  stay off the Homebrew `PATH`, and the formula test is valid.
- Windows build and test path handling: backslash `$0` in `go-product.sh`,
  preserved `TMP`, shell-normalized temp paths, `.exe` on the route-test
  binary, and a Windows-correct genie config check.
- `git` passthrough propagates the exit code and silences cobra errors.
- Sprint monitor tracks live readings on active resource alerts.
- Bash fixtures are gated with `LANG` unset.
- Judge write denial exits 126 after the effect-cap change (docs corrected).

#### Corrected

- Public claims now distinguish shipped Unix job control from the Windows
  process-group/TTY boundary, link yash evidence without an unsupported assertion
  percentage, and count 49 visible external commands from the pinned catalog.
- Build documentation requires Go 1.27 (toolchain go1.27.2) and states the
  CGo-free release claim the builds satisfy: no release links C — every
  platform, including the native Darwin job, builds `cmd/bashy` with
  `CGO_ENABLED=0`, and the product runtime build exposes the Go runtime's
  snapshot of inherited ignored signals to pure Go. See
  [claim evidence](docs/public-claims-evidence.md).

## [0.32.0] — 2026-10-07

One product from one tag: a single `bashy-<os>-<arch>` archive carries
`bashy`, `outpost`, `bash` and `sh`; `bashy self install [--service]` installs
them side by side; outpost no longer has its own release; `outpost upgrade`
applies the pair with rollback. Offline installs and seed caches.
Homebrew, Scoop and Winget manifests generated from the verified archives.
Built-in MCP server (policy-gated front-door verbs, session tools, HTTP
transport alongside stdio). Governed fleet fences. `bashy ycode` hosts the
native TUI over the builtin genie engine. The tree-sitter grammar set moved to
a blob-free fork and the GPL/MPL grammars were dropped; bashy stays
permissive-only. [Notes](docs/releases/v0.32.0.md).

## [0.31.0] — 2026-09-28

About 140 interpreted-Go engine changes from the Go 1.27 compiler-package work;
fixes a v0.30.0 regression that stopped the builtin genie agent; better live
steering of agent CLIs. [Notes](docs/releases/v0.31.0.md).

## [0.30.0] — 2026-09-27

`bashy llm`, the host's one model door (local models on bashy's own Ollama
engine plus the fleet's agent CLIs, served by band, model or agent name).
Contained calls (`@contain`, `bashy contain --net deny`). `bashy genie`,
`bashy app add/show/set/rm/edit`, `bashy stats`, `bashy ycode`. Exact GNU Bash
script-file error behaviour restored.
[Notes](docs/releases/v0.30.0.md).

## [0.29.0] — 2026-09-24

Clean-baseline release: one engine fix (Go-source programs can import the
standard library again) and every conformance number re-measured on one
reference revision on Linux, macOS and Windows.
[Notes](docs/releases/v0.29.0.md).

## [0.28.0] — 2026-09-23

Pinned shell engine update and cross-platform fixes exercised by the Go Tour,
Go by Example and Bash# Tour gates; named resource claims for shared-host
work. [Notes](docs/releases/v0.28.0.md).

## [0.27.0] — 2026-09-21

Bash# fences carry text artifacts (`dockerfile`, `tf`, `k8s`, `helm`, …) and
project manifests, and a runner of your own can process any fence.
[Notes](docs/releases/v0.27.0.md).

## [0.26.0] — 2026-09-21

Expanded opt-in Bash# Python and Rust runtime: persistent generators and
iterators consumable with `range`, TextIO filters that compose with byte
pipelines. [Notes](docs/releases/v0.26.0.md).

## [0.24.9] — 2026-09-20 · [0.24.2] — 2026-09-20 · [0.24.1] — 2026-09-19

Patch releases: team collaboration under GitHub's three team shapes
([0.24.9](docs/releases/v0.24.9.md)); `bashy check --bashsharp` null safety
inside a complete script ([0.24.2](docs/releases/v0.24.2.md)); a command
launched from a bashy prompt inherits the terminal
([0.24.1](docs/releases/v0.24.1.md)).

## [0.24.0] — 2026-09-18 · [0.23.0] — 2026-09-18

Bash++ is renamed **Bash#** (`bashsharp`), with `--bashsharp` / `.bsh` as the
front door and `--bashpp` / `.bpp` kept as deprecated aliases
([0.23.0](docs/releases/v0.23.0.md)).

## [0.22.0] — 2026-09-08

Bash# transpilation to standalone Go programs; shared resource observation for
sprint managers. [Notes](docs/releases/v0.22.0.md).

## [0.21.0] — 2026-09-06 · [0.20.0] — 2026-09-01

Earlier pre-1.0 releases; see the git history between the tags.

[Unreleased]: https://github.com/qiangli/bashy/compare/v1.0.0-rc.27...HEAD
[1.0.0-rc.27]: https://github.com/qiangli/bashy/compare/v0.32.0...v1.0.0-rc.27
[0.32.0]: https://github.com/qiangli/bashy/releases/tag/v0.32.0
[0.31.0]: https://github.com/qiangli/bashy/releases/tag/v0.31.0
[0.30.0]: https://github.com/qiangli/bashy/releases/tag/v0.30.0
[0.29.0]: https://github.com/qiangli/bashy/releases/tag/v0.29.0
[0.28.0]: https://github.com/qiangli/bashy/releases/tag/v0.28.0
[0.27.0]: https://github.com/qiangli/bashy/releases/tag/v0.27.0
[0.26.0]: https://github.com/qiangli/bashy/releases/tag/v0.26.0
[0.24.9]: https://github.com/qiangli/bashy/releases/tag/v0.24.9
[0.24.2]: https://github.com/qiangli/bashy/releases/tag/v0.24.2
[0.24.1]: https://github.com/qiangli/bashy/releases/tag/v0.24.1
[0.24.0]: https://github.com/qiangli/bashy/releases/tag/v0.24.0
[0.23.0]: https://github.com/qiangli/bashy/releases/tag/v0.23.0
[0.22.0]: https://github.com/qiangli/bashy/releases/tag/v0.22.0
[0.21.0]: https://github.com/qiangli/bashy/releases/tag/v0.21.0
[0.20.0]: https://github.com/qiangli/bashy/releases/tag/v0.20.0
