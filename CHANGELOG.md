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

## [1.0.0] — TBD (draft; not yet released)

Draft changelog entry for `v1.0.0`, summarizing user-visible changes merged to
`main` since `v1.0.0-rc.41` (through commit `1c89eb59`). Frozen candidate
commit, release date and final release evidence are `OPEN` and will be fixed on
the frozen `v1.0.0` candidate before promotion. Build coordinate on `main`:
`go 1.27.0`, toolchain `go1.27.4` (`go.mod`); Bash compatibility `5.3`. Draft
release notes (declared limitations, unsigned macOS/Windows binaries, supported
installs, Go interpreted performance disclosure, Go corpus inventory and
certification placeholder):
[docs/release-notes-v1.0.0-draft.md](docs/release-notes-v1.0.0-draft.md).

### Added

- **Offline self-contained operation (`bashy self`, `bashy self oci`, `bashy model offline`).**
  Added `bashy self bootstrap|seal|verify --offline|status|export|import|prune`
  over `yoke/pkg/artifact` and `yoke/pkg/gomod`, routing `self build` through
  the `productgo` runtime overlay when present and enforcing sealed network
  guards; added `bashy self oci capture|restore|build|lock-status|packs` with
  local CAS image restoration in `containEnsureImage` and `--pull=never
  --network=none` sealed builds; added `bashy model offline
  capture|restore|eval|unload|tier` for offline Ollama model management.
- **In-process `conform` judge and `selftest` verb split.** `bashy conform`
  dispatches directly to the pinned in-process `github.com/dhnt/conform` judge
  (including rule-by-example and lifecycle lint rules), while bashy's own
  verification batteries move to `bashy selftest` (`verify` stays as the
  battery alias; legacy entry points warn for one release).
- **Shapeware v0.1 / spec v2.2 layout and default KB runbooks.**
  `examples/desk/form` adopts the shapeware layout (`SKILL.md`, `CONTRACT.md`
  with assertions `F-01`–`F-20` and service rules, `features.yaml` with effort
  points and the poll feature, `dag.md` with band bounds and a defined `stop`
  step, `spec/` lifecycle, `form add --open` and `server.digest` verification);
  four linked shapeware runbooks ship in `kb/defaults/`.
- **Sprint worker and account status.** `bashy sprint` displays current worker
  and account readings and watches them for changes.
- **Graph stream `todo` fields.** Projects `known` todo status along with
  `workaround` and `expires` fields into the graph stream.
- **Security contact and v1.0.0 release note draft.** Added RFC 9116
  [`.well-known/security.txt`](.well-known/security.txt) (referenced from
  [`SECURITY.md`](SECURITY.md)) and draft v1.0.0 release notes at
  [`docs/release-notes-v1.0.0-draft.md`](docs/release-notes-v1.0.0-draft.md).

### Changed

- **Schedule engine state and engine-off hints.** `schedule list` leads with an
  engine-state line, `schedule add` and `notify --in/--at` hint on stderr when
  a job is stored while no schedule engine is running, and `kb/defaults/` ships
  the reminder runbook.
- **Dependency pins.** Synchronized `coreutils`, `yoke`, `bashsharp`,
  `outpost`, and `github.com/dhnt/conform` module pins in `go.mod` (including
  paired-host producer/adapter pins and coreutils updates).

### Fixed

- **Bundled `outpost` version stamp.** `scripts/dist.py` stamps the bundled
  `outpost` executable with the promoted distribution version instead of
  retaining a `-dev` suffix.
- **Release registration.** The release and promote workflows register each
  release with the hosted release index, so outposts update to it. The
  registration step required a macOS Intel outpost build, which is no longer
  built, and looked up the outpost commit on the GitHub API without a token.
  It failed for rc.40 and rc.41, so outposts were not told about either
  release. It now registers the five built platforms and passes the
  workflow token. A promoted release can be registered again with
  `gh workflow run promote.yml -f version=vX -f register_only=true`, which
  uses the workflow's current script, not the release's.
- **`bashy dag` on a fresh `HOME`.** `bashy dag` runs in-process on a fresh
  `HOME` without invoking `go`.
- **Desk `form`, `chart`, and `viewer` command handling.** Registered `form`
  and `chart` commands accept pass-through flags and isolate per-app stores,
  `form add` and `form ask` accept positional definition files,
  `viewer/register.sh` sets `GOWORK=off`, and `form stop` no longer fails when
  the stopping server has already removed `server.json`.

## [1.0.0-rc.41] — 2026-11-25

The first promoted release since `v1.0.0-rc.37`. `v1.0.0-rc.40-dev` was
published as a pre-release and never promoted (its Windows tier check
failed); its candidate notes below ship in this release together with the
fix. Build coordinate: Go 1.27, toolchain go1.27.4; Bash compatibility 5.3.

### Security

- Dependency bumps for advisories published after the rc.40 receipt:
  OpenTelemetry Go v1.47.0 (GO-2026-6974, GO-2026-6975), gobwas/ws v1.5.0
  (GO-2026-6940) and gorilla/websocket at the fixed 2025 revision
  (GO-2026-6278), in bashy, outpost, yoke and ycode.

### Fixed since the rc.40 candidate

- **Windows: Go adapters build on first use with a cold cache.** The
  dispatcher exports the adapter cache root in drive form, so the first-use
  build lands where the dispatcher looks for it. On rc.40, a fresh install
  without `BASHY_ADAPTER_CACHE` set failed the first run of every Go adapter
  on Windows.

### Added since the rc.40 candidate

- `bashy mcp serve --transport http --hosted`: a third serving profile beside
  `--outpost` and `--tools site`, for callers who have no host. Every request
  carries a master-signed, short-lived per-call identity; each executing call
  runs as a child process in a scratch home removed after it, under
  `--timeout` and `--max-output`; shell sessions are not served; the caller
  is the principal in the audit line. A gated call runs only after the
  caller approved it, and the caller's vault credentials are projected into
  the call. See `docs/mcp-server.md`.
- `bashy mcp connect`: bridges a local MCP client to an app on a satellite
  as a member. It exchanges only renewable, attenuated child tokens, keeps
  the owner's per-app decisions on every call, and reports gateway failures
  without reflecting response bodies.
- **Agent-ready sites.** Native site commands and the generic
  `<owner>/site` template: `/agents.txt`, `Link` headers, Markdown twins and
  JSON-LD; a site MCP tool profile (`list_pages`, `read_page`, `cite`, and
  owner-gated shape tools); verified bashy agents with signed site verbs,
  bound to the site's own origins. Members with the matching scope can pull
  a site's source and push a patch.
- **Shaping scopes.** `shape` enforces site member scopes, and owners get
  scope commands. The website example adds visitor personalization wishes
  (`wishes ls|add|rm`), a scope-gated member wish box, and follows the
  owner's approved revision. Access tiers use one vocabulary: anonymous,
  account, member, owner.
- **Desk page** (`examples/desk`): a meeting and workshop page on the desk
  verbs: join by short code, organizer, participant and display faces; poll,
  form, brainstorm, book, note, embed and timer blocks; live presence and a
  speaking queue; export to Markdown, HTML and PDF. Notepad, chart,
  calendar, viewer and brainstorm embed in a page, and page edits keep a
  revision history with undo.
- New examples: `release-mirror` (bashy as the LAN install server; it
  serves the outpost installers with `BASHY_RELEASE_BASE` pre-set) and
  `trove` (a virtual drive over Google Drive and GitHub).

## [1.0.0-rc.40] — 2026-11-22 (candidate, never promoted; ships in rc.41)

Everything user-visible merged since `v1.0.0-rc.37` (the last promoted
release). `v1.0.0-rc.38-dev` and `v1.0.0-rc.39-dev` were tagged but never
promoted (the rc.39 publication stopped at the release receipt), so their
draft notes are folded in below after the rc.40 changes. Build coordinate:
Go 1.27, toolchain go1.27.4; Bash compatibility 5.3.

### Changed

- **bashy is a thin core over one certified shell.** `bsh` (Bash 5.3, Bash#
  and the POSIX-required coreutils) is its own module at
  `bashsharp/cmd/bsh` and links no yoke code; bashy is the `bsh` engine, the
  agentic shell hooks, the internal verbs (`model`, `agent`, `tool`,
  `commands`, `app`, `skill`, `self`, `run`, `agentic` and their kin) and a
  dispatcher. Every other command lives in one of five folders
  (`optional/`, `toolchain/`, `yoke/`, `custom/`, `examples/`) as a
  Bash# adapter plus its own `main.go`, is built on first use into the
  adapter cache and run across the process boundary. The command surface is
  unchanged: `bashy <verb>` still works as before. `TestBashyImportBoundary`
  enforces that `cmd/bashy` links no command-folder package on Linux, macOS,
  Windows and the scratch and engines profiles.
- **The `bashy_cert_base` / `bashy_core` build profile is gone.** The
  certified thin shell is `bsh`; the Linux startup-time gate
  (`test-startup-linux`) now builds and measures `bsh` with unchanged
  thresholds. The `bashy_scratch` image ships the same thin core; adapted
  verbs need a Go toolchain and the adapter source on first use (decision
  record: `docs/decision-thin-core-command-delivery.md`).
- **Adapter scripts are embedded** in the bashy binary as the last fallback
  after a shape slot, `BASHY_ADAPTERS_ROOT`, the fetched source pin and an
  on-disk checkout.
- **Release receipts no longer accept a vulnerability override.** The
  `RELEASE_VULN_OVERRIDE` path, `vuln-override.json` and the `override`
  verdict were removed from `scripts/govulncheck-release.sh`,
  `scripts/local-release-evidence.py` and `release.yml`; a reachable finding
  blocks the receipt. Reachable advisories in bashy, outpost and yoke were
  bumped to fixed versions and `vuln-ignore` is empty.
- **Go 1.27.4** across the release modules; the official build image is
  `bashy-build:v1.0.0-rc.40-go1.27.4`.
- **`bashy dag --makefile`** generates the Makefile forwarder; `dag.md` is the
  only build and test source.

### Fixed

- **Adapter first use.** A cold adapter build under `BASHY_AGENTIC=1` no
  longer recurses into the `toolchain/go` adapter (fork bomb); adapter builds
  pass `-buildvcs=false` so a first `bashy sprint` inside a git hook with an
  inherited `GIT_DIR` succeeds; the adapter-trampoline environment
  (`BASHY_SELF` and friends) is stripped before any child process so `go
  test` binaries no longer inherit it.
- **Shell shims point at bashy, never at an adapter.** `install-agent` and
  `yoke/pkg/chat` resolve the shell from the bashy launcher identity
  (`BASHY_SELF`, then `bashy` on PATH) and refuse any target under the
  adapter cache. Before this, a foreman-managed session could rewrite
  `~/.bashy/shims/{bash,sh,zsh}` to exec the foreman binary.
- **`bashy dag`** mounts the sprint capacity services again after its move to
  the `yoke/dag` adapter; **`bashy kubectl`** on Windows keeps the
  `KUBECONFIG` path conversion (now covered by tests).
- **`bashy self install`** refuses to replace a newer installed bashy with an
  older release; **outpost** registers its service at boot from the bashy
  service bootstrap, so a service restore cannot put an older release over a
  newer build.
- `jq` is available to the standalone `yoke/dag` adapter.

### Added

- **`examples/update`**: one-command build and install of bashy for humans.
- **Demo Queue** example: timed team demos over the Outpost mesh.

### Earlier in this candidate (rc.38/rc.39 drafts)

Draft notes for 1.0.0-rc.39: everything user-visible merged since
`v1.0.0-rc.38-dev`, by theme. No changelog entries were cut for
rc.28–rc.38; the sprint-coordination verbs that landed during the rc.38
cycle are recorded here so the gap is not silent. Docs-only and test-only
commits are omitted except where they change a public claim.

#### Added

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

#### Changed

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

#### Fixed

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

[Unreleased]: https://github.com/qiangli/bashy/compare/v1.0.0-rc.41...HEAD
[1.0.0-rc.41]: https://github.com/qiangli/bashy/compare/v1.0.0-rc.27...v1.0.0-rc.41
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
