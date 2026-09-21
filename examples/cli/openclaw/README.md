# OpenClaw YAML CLI adapter

This profile captures a bounded common agent session lifecycle from the pinned
OpenClaw source: guided initialization, an interactive terminal session,
model discovery, and session resume, plus help, version, validation and
completion. It is a declarative `spec.interfaces.cli` projection compiled by
the generic ycode CLI contract — not a rewrite of OpenClaw.

Behavioral source pin: `ycode/priorart/openclaw` at
`6718352a63df815940c7a7bc5666f6484c7b8d9a`.

## Command surface

| Profile command | Pinned OpenClaw source | Status |
|---|---|---|
| `init` (alias `onboard`) | `onboard` — guided setup (`src/cli/program/register.onboard.ts`) | projected; adapter forwards as `onboard` |
| `start [target]` (aliases `tui`, `terminal`, `chat`) | `tui` + its `terminal`/`chat` aliases (`src/cli/tui-cli.ts`) | projected; adapter forwards as `tui`. TTY-only: a non-TTY invocation is a usage error (exit 2). Only `--session` is declared — the generic input route consumes it; upstream's `--message`/`--local` would be silently ignored by that route, so they are not declared and are rejected as usage errors (exit 2) |
| `model list` (alias `models`) | `models list` (`src/cli/models-cli.ts`) | supported against the compiled profile (prints configured models); adapter forwards as `models`. Upstream's `--all`/`--provider` would be silently ignored by the generic inspect dispatch, so they are not declared and are rejected as usage errors (exit 2) |
| `model status` | `models status` live auth/runtime probes | declared unsupported (exit 4): live probe state is not representable offline |
| `resume [query] [--handoff]` | `resume` (`src/cli/resume-cli.ts`) | projected; contract itself has no gateway sessions, so direct dispatch exits 4 and the adapter forwards to upstream |
| `plan` | none in the pinned source | declared unsupported (exit 4) in both profile and adapter |
| `exit` | none in the pinned source (sessions end inside the TUI) | declared unsupported (exit 4) in both profile and adapter |
| `version`, `validate`, `completion <shell>`, `help` | contract-standard operations | supported by the compiled contract |

Everything else in the pinned OpenClaw tree — gateway networking, channels,
nodes, devices, plugins, ClawHub, cron, docker, browser, hosted auth and
vendor wire protocols — is outside this bounded surface and is not claimed.
The contract's `completion` renders all four schema-required shells; the
pinned source itself ships bash and fish completion.

## Files

- `profile.yaml` — strict `spec.interfaces.cli` document; the sections outside
  `spec.interfaces.cli` (and `metadata.name`) are the shared runtime graph
  used by every profile in `examples/cli/` and carry no OpenClaw variation.
- `main.bsh` — thin native Bash++ adapter (typed `func`/`var`, no fenced Go —
  no helper needed one). It projects the bounded verbs onto the upstream
  spellings (`init`→`onboard`, `start`→`tui`, `model`→`models`), refuses
  `plan`/`exit` with exit 4 as declared, and otherwise execs the upstream
  executable named by `OPENCLAW_BIN` (default: `openclaw` on `PATH`),
  preserving argv, streams and exit status.
- `fixtures/run.sh` + `fixtures/golden/` — 39 offline fixture cases.

## What the YAML route runs (and what it does not)

`start` in the compiled profile dispatches into the **declared, neutral ycode
runtime graph and frontends** in this document (the shared `spec` sections
every `examples/cli/` profile carries) — it never launches OpenClaw. The only
path that reaches an upstream OpenClaw executable is the `main.bsh` Bash++
adapter, which execs `OPENCLAW_BIN`. The offline fixtures exercise that
adapter against a **fake** transport only, so the actual upstream OpenClaw
TTY and session behavior is not certified by this example.

## Validation

```sh
env YCODE_BIN=/abs/path/to/ycode/bin/ycode BASHY_BIN=/abs/path/to/bashy \
  /bin/sh examples/cli/openclaw/fixtures/run.sh
```

The runner uses a scratch working directory and makes no model calls.
31 cases drive the real ycode candidate against `profile.yaml`: strict
validation, golden root/nested/alias help (`fixtures/golden/*.txt`),
supported dispatch (`model list`, `version`, `completion bash`), declared
unsupported behavior (exit 4), usage errors (exit 2, including unknown
flag/command, argument arity, the completion shell enum, and the undeclared
upstream flags `start --message`, `start --local`, `model list --all`,
`model list --provider`), and stdin/TTY routing (`start` declares
`mode: auto, stdin: false`, so non-TTY invocations are rejected and piped
stdin is never consumed as a prompt). 8 cases drive `main.bsh` through the
installed Bashy against a **fake** upstream: they are honest transport
evidence only — argv projection, stdin/stdout passthrough and exit-status
propagation — and prove nothing about a real OpenClaw installation. Version
output is asserted non-empty rather than golden because it embeds the
candidate build string.
