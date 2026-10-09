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

### Corrected

- Public claims now distinguish shipped Unix job control from the Windows
  process-group/TTY boundary, link yash evidence without an unsupported assertion
  percentage, and count 49 visible external commands from the pinned catalog.
- Build documentation requires Go 1.27 with toolchain go1.27.1 and qualifies
  CGo-free release claims: Darwin `bashy` links a pre-Go C constructor to preserve
  inherited ignored signals. See [claim evidence](docs/public-claims-evidence.md).

Changes after v0.32.0, toward 1.0.0.

### Added

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

### Changed

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

### Fixed

- The installer enforces the corrected shell-pair contract; `bash` and `sh`
  stay off the Homebrew `PATH`, and the formula test is valid.
- Windows build and test path handling: backslash `$0` in `go-product.sh`,
  preserved `TMP`, shell-normalized temp paths, `.exe` on the route-test
  binary, and a Windows-correct genie config check.
- `git` passthrough propagates the exit code and silences cobra errors.
- Sprint monitor tracks live readings on active resource alerts.
- Bash fixtures are gated with `LANG` unset.
- Judge write denial exits 126 after the effect-cap change (docs corrected).

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

[Unreleased]: https://github.com/qiangli/bashy/compare/v0.32.0...HEAD
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
