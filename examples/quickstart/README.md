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
