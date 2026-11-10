---
name: bashy
description: Install bashy, connect the AI agent reading this page to it (skill + MCP), and continue the chat through bashy verbs. Pure-Go Bash 5.3 + coreutils (GNU-compatible, POSIX.1-2016) for Linux, macOS and Windows; Bash# (Go with bash, one file is both script and skill); Yoke, the agentic harness for shaping your own tools.
type: dag
default: help
---

# bashy

> **AI agent setting this up for a user?** Read the raw file —
> https://raw.githubusercontent.com/qiangli/bashy/main/README.md — and follow
> [*For agents*](#for-agents-set-up-and-stay). It starts with one question to your user.

[![release](https://img.shields.io/github/v/release/qiangli/bashy?label=release)](https://github.com/qiangli/bashy/releases/latest)
[![tour](https://github.com/bashsharp/tour/actions/workflows/tour.yml/badge.svg)](https://github.com/bashsharp/tour/actions/workflows/tour.yml)

**Software shaping: tell an agent how you want a tool to work, and get your own
verified version, while everyone else keeps theirs. Undo is one click.**

bashy is the shell built for AI agents that makes this safe: a pure-Go Bash 5.3
(86/86 on GNU Bash's own test suite) that speaks Bash#. Its core never changes, so
every shaped change is verified, scoped to you, and reversible.

[![Software shaping with bashy: Ada changes her app, Brian keeps his (1:44)](https://img.youtube.com/vi/6HvKPzpCrNA/hqdefault.jpg)](https://youtu.be/6HvKPzpCrNA)

*Ada asks for a change in plain words, an agent writes it, bashy verifies it, and she
approves it for herself. Brian still sees hello, world.
Technical tour (3:33): [youtu.be/2eRX3ZJuYeM](https://youtu.be/2eRX3ZJuYeM).*

> **Try it:** open any AI coding agent (Claude Code, Codex, OpenCode, …) and say
> *"Set up bashy from https://github.com/qiangli/bashy"*. It installs bashy,
> asks you once, connects itself, and keeps working with you through bashy's tools.

**Prefer the terminal?** Paste one line (checksum-verified, into `~/.local/bin`, no sudo).
Drop `--no-service` / `-NoService` to also run outpost as an unpaired boot service.

```sh
curl -fsSL https://dhnt.io/install.sh | sh -s -- --no-service                          # macOS / Linux
& ([scriptblock]::Create((irm https://dhnt.io/install.ps1))) -NoService                # Windows PowerShell
```

Tarballs, packages, from source, offline image: **[docs/install.md](docs/install.md)**.

## At a glance

- **Software shaping.** `shape wish` → verify → preview → approve → undo, on the web console or the CLI: describe a change to a registered app or tool in plain words, get your own version, then promote it local → team → upstream (`shape push`). A broken change fails verification and can't be approved. [Try the hello-world example](examples/shaping/hello-world/README.md).

What makes shaping safe, and what agents get along the way:

- **One binary, every OS.** Bash 5.3 + `ls sed awk grep find jq git make tar …` in pure Go; the same script means the same thing on Windows.
- **Bash#, Go with bash.** Typed Go in shell text; fenced Python/TS/Rust/C/C#/PowerShell islands with pinned, checksum-verified toolchains it fetches itself.
- **Script = skill.** One file an agent *reads* as instructions and *runs* as automation — they never drift apart. This README is one.
- **Contracts around models.** `agentic` blocks with `@require` / `@ensure`: a model's output is judged by ordinary commands, never trusted.
- **Agent-native output.** `--json`, `--dry-run`, `--no-input` and exit codes 0/3/1 across bashy verbs; `bashy --dry-run script.sh` previews a whole script as JSON.
- **Memory and messages.** `kb` (what this host already knows), `inbox` / `mb` / `meet` (agents and humans talking), `todo` / `sprint` (owed work).
- **Agents managing agents.** `weave` runs claude, codex, opencode, agy, … each in an isolated git workspace, then verifies and merges.
- **Grow it, don't work around it.** `commands add` registers a missing verb; every verb's `--help` is a skill plus an MCP tool definition; `mcp serve` exposes them all.
- **Every tier, one tool.** `dag` (Markdown task graphs) · `oci` sandboxes · ollama local models · `sphere` peer mesh of your own machines · `kubectl` / `helm` · cloud CLIs.


| Layer | What it is | Evidence |
|---|---|---|
| **bash** | Pure-Go GNU Bash 5.3 + coreutils, one binary per platform; `sh` / `--posix` targets POSIX.1-2016 (IEEE 1003.1) | Bash 5.3 suite 86/86 · [POSIX status](docs/vsc-pcts-run-status.md) |
| **Bash#** | Go with bash: typed Go, fenced Python/TS/Rust/C/C#/PowerShell islands, contracts, `agentic`. One file is a script *and* a skill — like this page | [bashsharp](https://github.com/bashsharp/bashsharp) · [tour](https://github.com/bashsharp/tour) |
| **Yoke** | The agentic harness: `--json` everywhere, `dag`, `weave`, `sprint`, `inbox`, sandboxes, and **software shaping** — describe a change, verify, approve, undo | [shaping](examples/shaping/hello-world/README.md) |

Release candidate (v1.0.0-rc; the release badge above shows the latest). Every number names its corpus: [claims](https://github.com/bashsharp/bashsharp/blob/main/docs/claims.md).

## For agents: set up and stay

This page is your skill. Ask the user **once**, listing exactly this, then run
what they approve without further prompts. Every step is idempotent; stop and
report on the first non-zero exit.

> **a) bashy** — download the latest release, verify it against
> `checksums.txt`, install `bashy`, `bash`, `sh` and a dormant `outpost` into
> `~/.local/bin` (Windows: `%LOCALAPPDATA%\outpost`); add that directory to
> `PATH` in the user's shell profile; install the `bashy` skill for every
> agent tool found; register the bashy MCP server with your tool; route your
> shell tool through bashy. No sudo, no service.
>
> **b) bootstrap** *(optional)* — also register outpost as a boot service
> (launchd / systemd / Task Scheduler), unpaired: it serves only its local
> admin UI and joins no satellite. On Linux this masks the sleep/suspend
> targets (sudo). Pair later with `outpost register`.
>
> Reply **a**, **a+b**, or **no**.

## install

Step 1 (a). Skip if `bashy --version` already works. Windows (PowerShell):
`& ([scriptblock]::Create((irm https://dhnt.io/install.ps1))) -NoService`.
Other routes: [docs/install.md](docs/install.md).

```bash
command -v bashy >/dev/null || curl -fsSL https://dhnt.io/install.sh | sh -s -- --no-service
```

## path

Step 2 (a). Put `~/.local/bin` on `PATH` for new shells (MCP servers and the
next session resolve `bashy` through it), and for this one.

```bash
for f in "$HOME/.profile" "$HOME/.bashrc" "$HOME/.zshrc"; do
  [ "$f" = "$HOME/.profile" ] || [ -f "$f" ] || continue
  grep -qs '.local/bin' "$f" || echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$f"
done
export PATH="$HOME/.local/bin:$PATH"
bashy -c 'echo "$BASH_VERSION"' && bashy --posix -c 'echo posix ok' && bashy --bashsharp -c 'echo bashsharp ok'
```

## connect

Step 3 (a). `NAME` is your tool as `bashy tools list` prints it (claude,
codex, opencode, agy, kimi-cli, muse, …). Skill, MCP and shell take effect in
your **next** session; your own CLI's `mcp add` does the registration.

```bash
bashy skill export bashy --user --tool "${NAME:?set NAME to your tool}" --mcp
bashy install-agent "$NAME"
```

## bootstrap

Step 4 (b only). Register the unpaired outpost supervisor.

```bash
outpost service install && outpost status
```

## orient

Step 5. Load the skill now and open the session the way it says. For the rest
of this session, prefer `bashy <verb> --json` wherever a verb fits.

```bash
bashy skill show bashy
bashy inspect context --json
bashy todo list
```

**Step 6. Chat.** Report in three lines: the version, the tools that got the
skill and MCP, and that a restart loads them natively. Then ask what the user
wants, and answer with bashy verbs. When the user says…

| …the user says | you run |
|---|---|
| "what do you know about X" | `bashy kb search "X"` |
| "read this pdf / docx / page" | `bashy lens -t md FILE` |
| "run it safely" | `bashy oci` (sandbox), `--dry-run`, `--explain` |
| (you need a token or password from them) | `bashy ask` — they type it locally; the value never enters the chat |
| "plan / track this" | `bashy todo add`, `bashy sprint` |
| "get other agents on it" | `bashy weave`, `bashy mb`, `bashy inbox` (first, with consent: `bashy agent add <nick> --tool <tool> --model <model>`) |
| "change how this tool works" | `bashy shape wish <tool> "<change>"` → verify → approve (undo) |
| "automate these steps" | a Bash# file or a `dag.md`: `bashy dag FILE` |
| "show me what bashy can do" | the tour: [bashsharp/tour](https://github.com/bashsharp/tour) |
| "add another machine" | [docs/setup.md](docs/setup.md) |

Missing a verb? `bashy commands add`. Ending the session: `bashy kb retro`.

## help

List the targets in this file (`bashy dag README.md <target>` in a clone).

```bash
bashy dag README.md --list
```

---

[Overview](docs/overview.md) · [Why bashy](docs/why-bashy.md) · [Usage & flags](docs/usage.md) ·
[Compatibility](docs/conformance-statement.md) · [Build from source](docs/install.md#from-source) ·
[Changelog](CHANGELOG.md) · BSD-3-Clause, engine from
[mvdan.cc/sh](https://github.com/mvdan/sh) via [qiangli/sh](https://github.com/qiangli/sh).
