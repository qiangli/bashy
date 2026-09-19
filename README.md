# bashy — a pure-Go Bash 5.3 that speaks Bash#

[![release](https://img.shields.io/github/v/release/qiangli/bashy?label=release)](https://github.com/qiangli/bashy/releases/latest)
[![tour](https://github.com/bashsharp/bashsharp-tour/actions/workflows/tour.yml/badge.svg)](https://github.com/bashsharp/bashsharp-tour/actions/workflows/tour.yml)

`bashy` is one static binary — no CGo, no system bash — that is a **drop-in
Bash 5.3** on Linux, macOS and Windows: same flags, same script semantics,
same `$BASH_VERSION`, and it passes GNU Bash's own 5.3 test suite (every
runnable fixture, 86/86). With `--bashsharp` the same binary speaks
**[Bash#](https://github.com/bashsharp/bashsharp)**: the bash you already know,
Go where you need types, any fenced language where you need a library, and
`agentic` where you need a model — with contracts so a model's output is
judged, never trusted.

> **Alpha** (0.x). Bash 5.3 compatibility is stable; the Bash# dialect may
> still change before 1.0 through RFCs. Every number this project states
> names its corpus: [docs/claims.md](https://github.com/bashsharp/bashsharp/blob/main/docs/claims.md).

## Ten minutes

```sh
# macOS (Apple Silicon)
curl -fsSLO https://github.com/qiangli/bashy/releases/latest/download/bashy-darwin-arm64.tar.gz
tar -xzf bashy-darwin-arm64.tar.gz && sudo install bashy /usr/local/bin/bashy
bashy --version
```

Then take the tour — 27 small programs with pinned transcripts and one
script that runs them all on your machine, also written as a procedure your
coding agent can drive: **[bashsharp/bashsharp-tour](https://github.com/bashsharp/bashsharp-tour)**.
The ten-minute version is in this repo: [`examples/quickstart/`](examples/quickstart/).

```bash
@guard(effects: "read")
@require('test -n "$1"')
@ensure('test "$1" != lie')
agentic function summarize() { ... }

agentic {
    summarize ok        # exit 0
    summarize ""        # exit 3 — precondition failed; the body never ran
    summarize yield     # exit 6 — "I need input": a yield, not a made-up answer
}
```

The interpreter never calls a model. `agentic` marks the one place a program
may hand work to one, and the contracts around it are ordinary shell
commands, run deterministically.

## What you get

- **A Bash 5.3 you can ship anywhere.** One binary per platform; job control,
  coprocesses, signal traps, locale-aware globbing — verified against Bash's
  own suite on Linux and macOS. Invoked as `sh`, or with `--posix`, it is a
  POSIX shell: 493/493 on the licensed VSC shell arm, 99 %+ on yash's POSIX
  suite (bash 5.3 itself scores 96 % there).
- **The pure-Go userland with it.** `ls`, `sed`, `awk`, `grep`, `find`,
  `sort`, `tar`, `jq`, `git`, `make`, … as applets, so the same script means
  the same thing on Windows.
- **Bash#** with the flag on: typed Go in shell text, fenced Python /
  TypeScript / Rust / C / C++ / Go islands, decorators, keyword arguments,
  enums, deep `readonly`, contracts and `agentic`. Off with `--no-bashsharp`
  or `--posix`, where none of it exists.
- **The islands bring their own toolchains.** A fence never resolves its
  tool from your `PATH`: bashy provisions what it uses — Go 1.27.1, `zig cc`
  for C/C++, a uv-managed CPython, Node + `typescript`, a rustup toolchain —
  downloaded from the vendor once, checksum-verified against a pin in this
  repo, cached — so the same program means the same thing on every machine.
  `bashy check --prepare SCRIPT...` pays that download ahead of time;
  `BASHPP_PYTHON`, `BASHPP_GO`, `BASHPP_CC`, … name a program explicitly.
- **It rebuilds itself.** `bashy git clone`, `bashy scripts/bootstrap-siblings.sh`,
  `bashy dag build` — on Windows with no git, no Go and no C compiler on the
  host (see *From source*).
- **A tool that knows its caller is an agent.** `bashy check` for static
  checks, `bashy dag` for dependency-ordered tasks in Markdown, `bashy awd`
  for "run this there", registered commands, and the `agentic` yield status
  a harness can act on.

Built on the [`qiangli/sh`](https://github.com/qiangli/sh) fork of
[`mvdan.cc/sh`](https://github.com/mvdan/sh) by Daniel Martí — the engine is
his; the Bash 5.3 conformance work and the Bash# dialect are carried in the
fork. Campaign identity, regression gates and the product sequence:
[docs/internal/campaign-and-gates.md](docs/internal/campaign-and-gates.md).

## Install

### Download a release binary

Grab the archive for your platform from the
[Releases](https://github.com/qiangli/bashy/releases) page and put `bashy` on
your `PATH`:

| Platform | Asset |
| --- | --- |
| Linux x86-64 | `bashy-linux-amd64.tar.gz` |
| Linux arm64 | `bashy-linux-arm64.tar.gz` |
| macOS Intel | `bashy-darwin-amd64.tar.gz` |
| macOS Apple Silicon | `bashy-darwin-arm64.tar.gz` |
| Windows x86-64 | `bashy-windows-amd64.zip` |
| Windows arm64 | `bashy-windows-arm64.zip` |

```sh
# Linux/macOS example
tar -xzf bashy-linux-amd64.tar.gz
sudo install bashy /usr/local/bin/bashy
bashy --version
```

### With Go

`go install github.com/qiangli/bashy@latest` is **not** supported: the module
resolves its engine and siblings through flat `replace ../<sibling>`
directives, which `go install` refuses. Use a release archive above, or build
from source below.

### From source

bashy rebuilds itself using only an installed bashy. Every command below is
run *through* `bashy`, so the same five lines work on Linux, macOS and
Windows: `bashy git` fetches the sources, `bashy scripts/bootstrap-siblings.sh`
checks out the sibling modules at the exact SHAs in `.sibling-pins`, and
`bashy dag build` compiles both binaries through `bashy go`, which downloads
and verifies its own pinned Go toolchain into bashy's cache the first time.

```sh
bashy git clone https://github.com/qiangli/bashy
cd bashy
bashy scripts/bootstrap-siblings.sh
bashy dag build            # -> bin/bash and bin/bashy (bin/*.exe on Windows)
bashy dag install          # optional: install into ~/.local/bin ($DHNT_BIN_DIR to change)
```

What the host must provide, per platform:

| | git | Go | C compiler |
| --- | --- | --- | --- |
| Windows | **none** — `bashy git` downloads a pinned, checksum-verified MinGit | **none** — `bashy go` provisions it | not used |
| macOS | the system `git` (Xcode Command Line Tools: `xcode-select --install`) | **none** — `bashy go` provisions it | optional |
| Linux | the distribution's `git` | **none** — `bashy go` provisions it | optional |

The C compiler is optional on Linux and macOS: with `cc` on `PATH` the build
also compiles the native pre-Go signal launcher (`bin/bashy` + `bin/bashy.real`);
without one it says so and ships the plain Go binaries — the same form the
release archives ship. `bashy git` on Linux and macOS deliberately uses the
platform git rather than downloading one.

A checkout that has no installed bashy yet can bootstrap from the repo-local
launcher instead (Linux/macOS; it needs a host `go`):

```sh
./bashy dag build
./bashy dag install
```

The traditional host-tool path also works when `git`, `go` and `make` are
already installed:

```sh
git clone https://github.com/qiangli/bashy
cd bashy
./scripts/bootstrap-siblings.sh    # clones each sibling next door at its pinned SHA
make build                         # -> bin/bash and bin/bashy
```

A minimal Linux container base (Ubuntu/glibc, launcher + payload) is described
in [`docs/bashy-oci-base.md`](docs/bashy-oci-base.md).

Real-repository examples driven by `bashy dag` — one `dag.md` each for
GitHub CLI, Hugo, Caddy, curl, git, FFmpeg, tesseract, llama.cpp, CMake, uv,
Codex, Bun, OpenCode, OpenClaw, Hermes Agent and more, calling their Go,
Python, TypeScript, Rust and C/C++ code as fenced islands — live under
[`examples/dag/`](examples/dag/).

## Usage

```sh
bashy script.sh arg1 arg2      # run a script
bashy -c 'echo "$BASH_VERSION"'# run a command string
bashy                          # interactive shell
echo 'echo hi' | bashy         # read a script from stdin
```

### Supported flags

`bashy` accepts the common Bash invocation flags:

| Flag | Meaning |
| --- | --- |
| `-c <string>` | run `<string>` as a command |
| `-i` | force interactive mode |
| `-l`, `--login` | act as a login shell |
| `--posix` | POSIX mode |
| `--norc` | do not read `~/.bashyrc` |
| `--noprofile` | do not read profile files |
| `--rcfile`, `--init-file <f>` | use `<f>` as the interactive startup file |
| `-o <opt>` | enable a `set` option (e.g. `errexit`, `xtrace`) |
| `-O <opt>` | enable a `shopt` option |
| `--pretty-print` | pretty-print the parsed input |
| `--version` | print version and exit |

### Shell mode selection

Invoked as `bash` or `bashy`, the shell starts in GNU Bash 5.3-compatible
mode. Invoked with basename `sh`, it starts in POSIX `sh` mode. POSIX mode can
also be requested with `--posix`, `-o posix`, `SHELLOPTS=posix`, or by the
presence of `POSIXLY_CORRECT`/`POSIX_PEDANTIC` (including empty values).
Command-line `-o/+o posix` is last-wins unless one of the environment or `sh`
startup conditions forces POSIX mode, matching GNU Bash 5.3.

The complete contract, including strict `sh` semantics and certification
wiring, is documented in [Shell mode selection](docs/shell-mode-selection.md).

Startup files: interactive shells read `~/.bashyrc` (or `--rcfile`); login
shells read `/etc/profile` and `~/.bashy_profile`; `$BASH_ENV` is honoured for
non-interactive shells.

## Compatibility notes

`bashy` is a pure-Go runner: subshells are goroutines rather than `fork()`,
and process substitutions use real named pipes. Job control
(`jobs`/`fg`/`bg`/`kill %n`/`suspend` with stopped-state tracking),
coprocesses, and signal traps are implemented and pass Bash's test suite on
Unix. Mirroring Bash's own design (`jobs.c` on Unix, `nojobs.c` elsewhere),
the OS-level job-control machinery is Unix-only; on other platforms it
degrades exactly as a no-job-control Bash does.

Two known gaps: arithmetic currently uses the native int width (64-bit on
64-bit platforms), so very large values on 32-bit builds truncate — a tracked
int64 migration; and the Windows test-suite run is still being verified.
Everything else — parameter expansion, arrays and associative arrays,
namerefs, `[[ ]]`, arithmetic, here documents, brace/tilde/glob expansion
(locale-aware, including non-UTF-8 charsets such as Big5/Shift-JIS), traps,
`printf`, `read`, prompt escapes — matches Bash 5.3 and is verified against
Bash's own test suite.

## Development

See [`CLAUDE.md`](CLAUDE.md) for the development workflow and [`docs/`](docs/)
for the compliance roadmap and per-fixture analyses. The Bash 5.3 suite is
driven by `make test-bash` (serial; needs a controlling terminal; `make
test-bash-fixtures` fetches the pinned fixture tree). The language, its
roadmap and RFCs live in [bashsharp/bashsharp](https://github.com/bashsharp/bashsharp).

## License

BSD 3-Clause (inherited from `mvdan.cc/sh`). See [`LICENSE`](LICENSE).
