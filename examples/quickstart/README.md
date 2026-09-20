# Bash# in ten minutes

Bash# is **alpha**. Everything in this directory runs today on a released
`bashy`; nothing here needs a model, an API key, a network, or a Go toolchain.
Syntax may still change before 1.0 — the way to influence it is an RFC (see
`ROADMAP.md` in the language repo).

## Install

Download the archive for your platform from the bashy Releases page, unpack
it, and put `bashy` on your `PATH`. Then:

```sh
bashy --version
```

## Run the demo

```sh
bashy --bashsharp judge.bsh
```

`judge.bsh` is one `agentic` function under three deterministic contracts:

| call | what happens | exit |
|---|---|---|
| `summarize ok` | require passes → body runs → ensure passes | **0** |
| `summarize ""` | `@require` fails; the body never runs | **3** |
| `summarize lie` | body runs and exits 0; `@ensure` disagrees | **3** |
| `summarize write` | body tries to write under `@guard(effects: "read")` | **1** (denied) |
| `summarize fail` | body returns 1 | **1** |
| `summarize yield` | body returns 6 — *input required* — no ensure runs | **6** (a yield) |

Exit 6 is the interesting one: the interpreter never calls a model. An
`agentic` body that needs something it does not have *yields* to whoever is
running the script — your shell, or the coding agent that ran `bashy -c` —
so the agent can ask, and retry. The same `@ensure` then guards a typed Go
function in the same file (`twice`).

`judge.expected` is the pinned transcript; `./check.sh` diffs every example
in this directory against its transcript on the `bashy` on your `PATH`
(pass a path to test another binary).

**The full tour** — six chapters, 27 programs, a version for coding agents
(`SKILL.md`), and a gate that runs on every OS against the latest release —
lives in its own repo: [bashsharp/tour](https://github.com/bashsharp/tour).

## Three delivery modes (Sprint 216, Story 540)

Beyond the interpreter, Bash# scripts can be delivered in three ways from scratch:

### (a) bashy + .bsh — interpreted, no toolchain

`hello.bsh` is the minimum executable example. It needs nothing beyond the
installed `bashy` binary:

```sh
bashy --bashsharp hello.bsh        # explicit flag
bashy hello.bsh                    # .bsh implies --bashsharp
```

### (b) `transpile --standalone` — native Go binary, no bashy at runtime

`hello_standalone.bsh` transpiles to a native binary that runs without bashy,
a container engine, or any external provider. **Smaller supply-chain surface**:
the standalone binary imports only `mvdan.cc/sh/v3/lower/shellrt` (the plain
runtime base) and the Go standard library — it does NOT include bashy, podman,
ollama, gh, loom, act, rclone, zot, seaweedfs, or searxng.

> **Note:** "smaller supply-chain surface" describes dependency reduction.
> The binary is NOT sandboxed — it runs as the user with standard OS permissions.

```sh
out=$(mktemp -d)
bashy transpile --bashsharp hello_standalone.bsh --standalone -o "$out/main.go"
cd "$out" && GOPROXY=direct GONOSUMDB='*' go mod tidy
go build -o hello .               # pure Go, no interpreter dependency
./hello                           # runs without bashy on PATH
go version -m ./hello             # inspect SBOM: one shell-runtime dep
```

`check --prepare` is **not** needed here — the build requires Go and network
access to resolve the shell-runtime module; `go mod tidy` handles both.

### (c) Pre-prepared Python island — no runtime download

`hello_island.bsh` embeds a Python function. Provision the toolchain once
(on the target host, in a Dockerfile layer, or in CI before going air-gapped):

```sh
bashy check --prepare hello_island.bsh   # provisions python3; cache-first
bashy --bashsharp hello_island.bsh       # runs offline after preparation
```

`check --prepare` is idempotent: a second call downloads nothing. The
provisioned python3 is reused for every subsequent run on that host. No
`pip install` is needed — the island uses only the Python standard library.

### Smoke gate

```sh
make smoke-quickstart              # all three modes, local
SKIP_CONTAINER=1 make smoke-quickstart   # suppress optional container leg
```

The local smoke is deterministic and authoritative. The container leg records
compressed/uncompressed binary sizes and a `go version -m` SBOM line when
docker or podman is available, but its absence does not fail the gate.

## The other files

Each is copied from a fixture in the conformance suite and runs the same way:

- `decorators.bsh` — `@tag(...)` lines above a declaration; a decorator is an
  ordinary function taking `c *Call`, and its arguments use keyword and
  default parameters, evaluated per call.
- `kwargs.bsh` — `func greet(name string, retries int = 3)`,
  `greet(retries: 5, name: "Bob")`.
- `enums.bsh` — `type Color enum { Red; Green }` and a `switch` that must
  cover every member.
- `readonly.bsh` — bash's own `readonly`, extended to freeze a struct, slice or
  map all the way down, through aliases and subshells.

## What Bash# is

The bash you already know (every Bash 5.3 script means the same thing; with
the flag off the dialect is inert), Go where you need types, any fenced
language where you need a library, and `agentic` where you need a model —
with contracts so a model's output is judged, never trusted.

Every number about Bash# names its corpus, and lives in the language repo's
`docs/claims.md`.
