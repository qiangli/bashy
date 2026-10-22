# bashy — a Go Bash 5.3 that speaks Bash#

[![release](https://img.shields.io/github/v/release/qiangli/bashy?label=release)](https://github.com/qiangli/bashy/releases/latest)
[![tour](https://github.com/bashsharp/tour/actions/workflows/tour.yml/badge.svg)](https://github.com/bashsharp/tour/actions/workflows/tour.yml)

> **Bash is for humans; bashy is for agents.**
>
> bashy looks like a shell, and it is one. It was also designed from the
> ground up as a clean slate for agents: the whole infrastructure an agent
> needs, shipped as a single tool:
> - the userland it works in;
> - the workspaces it shares with other agents;
> - the sandboxes it runs untrusted code in;
> - the local models it thinks with;
> - the peers, clusters and clouds it reaches.
>
> One download, every machine, every agent. **bashy is all you need.**

- **bash + coreutils in pure Go**: one executable per platform, no system bash,
  no GNU-vs-BSD drift.
- **GNU Bash 5.3 compatible**: Bash's own 5.3 suite, 86/86.
- **Built to POSIX.1-2016 (IEEE Std 1003.1-2016)**: 493/493 on the licensed
  conformance suite; formal certification is in progress.
- **Bash#, "Go with bash"**: the bash you know, typed Go where you need it.
  Skills and scripts are two sides of the same coin: read it as a skill, run
  it as a script.
- **Yoke, bashy's agentic shell harness**: the name is a backronym, *Bashy's
  Agentic Shell Harness Yoke*.

`bashy` is one executable per platform, with no system bash dependency — a **drop-in
Bash 5.3** on Linux, macOS and Windows: same flags, same script semantics,
Bash-compatible `$BASH_VERSION`, and it passes GNU Bash's own 5.3 test suite (every
runnable fixture, 86/86). With `--bashsharp` the same binary speaks
**[Bash#](https://github.com/bashsharp/bashsharp)**: the bash you already know,
Go where you need types, any fenced language where you need a library, and
`agentic` where you need a model — with contracts so a model's output is
judged, never trusted.

> **Alpha** (0.x). Bash 5.3 compatibility is stable; the Bash# dialect may
> still change before 1.0 through RFCs. Every number this project states
> names its corpus: [docs/claims.md](https://github.com/bashsharp/bashsharp/blob/main/docs/claims.md).

Deciding whether to use it, or which part of it is for you: [docs/why-bashy.md](docs/why-bashy.md).

The shell and userland are Go. Release builds use `CGO_ENABLED=0` on Linux,
macOS and Windows; inherited ignored signals are preserved by bashy's product
runtime build, not by C. The Linux `bashy` still loads glibc dynamically for its
locale gate. See [build evidence](docs/public-claims-evidence.md).

## Ten minutes

```sh
# macOS (Apple Silicon)
curl -fsSLO https://github.com/qiangli/bashy/releases/latest/download/bashy-darwin-arm64.tar.gz
tar -xzf bashy-darwin-arm64.tar.gz
# for your user (no sudo):
mkdir -p ~/.local/bin && install bashy ~/.local/bin/bashy && export PATH="$HOME/.local/bin:$PATH"
# or system-wide, if you are an administrator:
#   sudo install bashy /usr/local/bin/bashy
bashy --version
```

Then take the tour — 27 small programs with pinned transcripts and one
script that runs them all on your machine, also written as a procedure your
coding agent can drive: **[bashsharp/tour](https://github.com/bashsharp/tour)**.
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
  POSIX shell: 493/493 on the licensed VSC shell arm (milestone evidence).
  For yash, see [yash-chunks.json](yash-chunks.json) and the
  [measurement boundaries](docs/public-claims-evidence.md); fixture completion
  is not an assertion pass percentage.
- **The pure-Go userland with it.** `ls`, `sed`, `awk`, `grep`, `find`,
  `sort`, `tar`, `jq`, `git`, `make`, … as applets, so the same script means
  the same thing on Windows.
- **Bash#, Go with bash.** With the flag on: typed Go in shell text, fenced Python /
  TypeScript / Rust / C / C++ / Go / C# / PowerShell islands, decorators, keyword arguments,
  enums, deep `readonly`, contracts and `agentic`. Off with `--no-bashsharp`
  or `--posix`, where none of it exists.
- **Skills and scripts: two sides of the same coin.** In Bash# they are one
  object with two orientations. A skill is prose with fenced code you can run;
  a script is code with prose around it. An agent **reads** a Bash# file as a
  skill (what to do, why, with which tools) and **runs** the same file as a
  script, so the instructions and the automation never drift apart. (More in
  the post [A Bash# script is a skill](https://www.linkedin.com/feed/update/urn:li:activity:7516083010736181248/).)
- **The islands bring their own toolchains.** A fence never resolves its
  tool from your `PATH`: bashy provisions what it uses — Go 1.27.2, `zig cc`
  for C/C++, a uv-managed CPython, Node + `typescript`, a rustup toolchain, and one PowerShell 7
  runtime that serves both the `powershell` and `csharp` fences —
  downloaded from the vendor once, checksum-verified against a pin in this
  repo, cached — so the same program means the same thing on every machine.
  `bashy check --prepare SCRIPT...` pays that download ahead of time;
  `BASHPP_PYTHON`, `BASHPP_GO`, `BASHPP_CC`, … name a program explicitly.
- **It rebuilds itself.** `bashy git clone`, `bashy dag build` — on Windows with no git, no Go and no C compiler on the
  host (see *From source*).
- **A tool that knows its caller is an agent.** `bashy check` for static
  checks, `bashy dag` for dependency-ordered tasks in Markdown, `bashy awd`
  for "run this there", registered commands, and the `agentic` yield status
  a harness can act on. **Rod, not fish:** every command is small, composable
  and scriptable (`--json`, `--help` that doubles as a skill), so an agent
  builds what it needs instead of waiting for a feature.
- **One tool, every tier.** The same binary drives every tier:
  - the userland and workplace agents collaborate in (`weave`, `sprint`, `inbox`);
  - OCI sandboxes (`bashy oci`);
  - local models through ollama;
  - the sphere, a peer-to-peer mesh of your own machines (their outposts);
  - Kubernetes clusters built from those same machines;
  - the cloud provider of your choice.

  It is more than a bash: it is the agent's whole infrastructure, delivered as
  a single tool. See *Layers, tiers and the o-zone* below.

## Layers, tiers and the o-zone

Three words locate any bashy feature. A **layer** says *what language or tool
surface* you are using, a **tier** says *where the work runs*, and the
**o-zone** names the three runtime services every tier leans on.

**Layers: bash → Bash# → bashy.** Each layer is cumulative, and a higher one
may never weaken a lower one.

| Layer | What it is | Also called |
|---|---|---|
| **bash** | GNU Bash 5.3 compatibility, POSIX.1-2016 conformance, and the pure-Go coreutils userland | Classic |
| **Bash#** | Go with bash: typed Go, fenced Python / TypeScript / Rust / C / C++ / C# / PowerShell islands, contracts, `agentic` | the language |
| **bashy** | the agentic shell harness: tools, policy, durable sessions, coordination, workflow, observability | Yoke |

**Tiers: userland → workplace → sandbox → sphere → cluster → cloud.** A
command has one primary tier; userland tools work in every tier.
The sphere and the cluster are not someone else's cloud: both are built from
the hosts you own. Each machine runs an outpost; the outposts form the
peer-to-peer sphere, and the same outposts join as nodes of a standard
Kubernetes cluster.

| Tier | Primary purpose | Examples |
|---|---|---|
| **userland** | POSIX/coreutils, classic utilities, toolchains | `bashy`, `sed`, `awk`, `jq`, `make` |
| **workplace** | agent work and collaboration | `weave`, `dag`, `sprint`, `inbox` |
| **sandbox** | OCI containers | `bashy oci` (`sandbox`) |
| **sphere** | a peer-to-peer mesh formed by the outposts on the machines you own: they share capabilities with each other directly: pooling their GPUs and local models into one LLM endpoint, and sharding a model too large for any one machine across several of them | `bashy sphere` |
| **cluster** | a standard Kubernetes cluster (DKS) whose nodes are those same outposts, plus a satellite as control plane; run ordinary Kubernetes workloads on your own hardware | `bashy kubectl`, `helm` |
| **cloud** | external providers | `bashy doctl`, `aws`, `gcloud` |

**The o-zone (O3): otel · ollama · oci.** The extended runtime beyond
coreutils: **otel** for traces and metrics on every run, **ollama** for local
models, **oci** (podman) for sandboxes. bashy manages all three for you. The
separately licensed programs (podman, ollama) are downloaded and checksum-verified
on first use, never bundled.

## bashy, outpost and cloudbox

bashy is one of three pieces that together let agents operate **easily,
safely, securely, and with each other**, on the machines you already own:

- **bashy**, the agent's shell and toolbox, on every machine.
- **[outpost](https://github.com/qiangli/outpost)**, the host agent. It runs
  on your laptops, desktops and servers, serves the apps you choose to expose,
  and pairs each machine with a satellite so your fleet can reach it.
- **cloudbox**, the coordination plane, offered as the hosted service
  [Tessaro](https://tessaro.sh). It handles identity, scoped short-lived
  tokens, grants, satellites, and the metadata needed to rebuild everything.
  It **coordinates; it does not host your work.**

Two rules shape all three:

- **Local first.** Compute, GPUs, storage and connections are yours: your
  machines, your satellites, your choice of cloud or S3 bucket. The cloud is a
  thin, replaceable relay.
- **Rod, not fish.** bashy ships simple, composable tools (`--json`,
  `--dry-run`, idempotent, credentials from a vault), not one-off answers.
  Named providers in the docs are examples; an agent can wire any backend the
  tools reach.

Built on the [`qiangli/sh`](https://github.com/qiangli/sh) fork of
[`mvdan.cc/sh`](https://github.com/mvdan/sh) by Daniel Martí — the engine is
his; the Bash 5.3 conformance work and the Bash# dialect are carried in the
fork. Campaign identity, regression gates and the product sequence:
[docs/internal/campaign-and-gates.md](docs/internal/campaign-and-gates.md).

Python packages used across the repos, with CPython 3.15 wheel readiness:
[docs/python-package-inventory.md](docs/python-package-inventory.md). Refresh it with
`DHNT_ROOT=<umbrella checkout> bashy python scripts/python-package-inventory.py`.

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
mkdir -p ~/.local/bin && install bashy ~/.local/bin/bashy   # put ~/.local/bin on PATH
# or system-wide, if you are an administrator: sudo install bashy /usr/local/bin/bashy
bashy --version
```

A prerelease (a `-rc.N-dev` tag) is not `latest`: download it from its tag,
`https://github.com/qiangli/bashy/releases/download/<tag>/<asset>`.

### With Go

`go install github.com/qiangli/bashy@latest` is **not** supported: the module
uses `replace` directives for its forks (the `mvdan.cc/sh/v3` engine and a
few vendored upstreams), which `go install` refuses. Use a release archive above, or build
from source below.

### From source

bashy rebuilds itself using only an installed bashy. Every command below is
run *through* `bashy`, so the same five lines work on Linux, macOS and
Windows: `bashy git` fetches the sources, and `bashy dag build` compiles both binaries through `bashy go`, which downloads
and verifies its own pinned Go toolchain into bashy's cache the first time.
The sibling modules are go.mod pins, so the go command downloads exactly the
versions this commit names.

```sh
bashy git clone https://github.com/qiangli/bashy
cd bashy
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
make build                         # -> bin/bash and bin/bashy
```

### Containers — the offline image

Bashy is all you need. With nothing but the release download — no git, go,
podman or docker on the host — build the image and run your script with no
network:

```sh
bashy self image
bashy podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:<ver>-linux-<arch> --bashsharp ./script.bsh
```

`bashy self image` fetches the release's static `bashy-scratch-linux-<arch>`
artifact (checksum-verified) and builds a `FROM scratch` image around it
through `bashy podman` — the engine bashy provisions for itself from the
pinned upstream releases (a complete static podman on Linux; the machine
client plus gvproxy/vfkit on macOS; the client on Windows, where the machine
runs on WSL2 — every edition). The image is bashy as it is: Bash 5.3, `--posix`,
Bash#, the builtin coreutils, dag/weave/check/transpile. What is and is not in
it, per command, is the measured matrix in
[`docs/airgap-image.md`](docs/airgap-image.md). From a checkout,
`bashy dag build-image` images the candidate instead of a published artifact.

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
Unix process groups and controlling-terminal handoff are implemented. Windows
does not provide Unix process-group/TTY job control; its basic job carrier
provides process identity but no live signal proxy (`os/exec` rejects
`ExtraFiles`). See [job-carrier.md](docs/job-carrier.md).

Arithmetic currently uses the native int width (64-bit on 64-bit platforms),
so very large values on 32-bit builds truncate — a tracked int64 migration.
Unix job control has shipped; the remaining interactive boundaries are listed
in [the conformance statement](docs/conformance-statement.md#declared-limitations).
The final Sprint 253 production gate measured the Bash 5.3 fixtures at
**86/86 on Windows and Linux** (run 35812307698), and the Sprint 257 timezone
follow-up measured **86/86** on two native Windows builds, native macOS, and
Ubuntu 24.04 test droplets. These are fixture results, not a claim of full
Bash compatibility; see the umbrella's `docs/sprint-253-delivery-evidence.md`.
Everything else — parameter expansion, arrays and associative arrays,
namerefs, `[[ ]]`, arithmetic, here documents, brace/tilde/glob expansion
(locale-aware, including non-UTF-8 charsets such as Big5/Shift-JIS), traps,
`printf`, `read`, prompt escapes — matches Bash 5.3 and is verified against
Bash's own test suite.

## Development

Builds require `go 1.27` and select `toolchain go1.27.2`, as recorded in
[`go.mod`](go.mod). Product versions come from the release tag; use
`bashy --version` to identify the installed build.

See [`CLAUDE.md`](CLAUDE.md) for the development workflow and [`docs/`](docs/)
for the compliance roadmap and per-fixture analyses. The Bash 5.3 suite is
driven by `make test-bash` (serial; needs a controlling terminal; `make
test-bash-fixtures` fetches the pinned fixture tree). The language, its
roadmap and RFCs live in [bashsharp/bashsharp](https://github.com/bashsharp/bashsharp).

## License

BSD 3-Clause (inherited from `mvdan.cc/sh`). See [`LICENSE`](LICENSE).
