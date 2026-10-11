# Why bashy, and for whom

Read this page before you adopt bashy, and give it to whoever has to approve
that. bashy is **alpha (0.x)**. Every feature below is marked **shipped** or
**planned**. Each number links to the corpus it was measured on, in
[public-claims-evidence.md](public-claims-evidence.md) or the Bash#
[claims.md](https://github.com/bashsharp/bashsharp/blob/main/docs/claims.md).
A number that is in neither file does not belong on this page.

## The one-screen version

**bashy is one permissively licensed binary. It gives every machine the same
Bash 5.3 and the same Unix userland: Windows, macOS, Linux, or an empty
container. Your coding agents share it as a userland.** It is built in layers,
and you can stop at any one of them:

1. **bash, the same everywhere.** A pure-Go Bash 5.3 with the coreutils built
   in. One executable per platform, with an audit trail, dry runs and telemetry
   you can turn on.
2. **Bash#, one front door for a polyglot repository.** A `dag.md` task graph,
   and Python, TypeScript, Rust, C/C++, Go, C# or PowerShell fenced into one
   script. Each language runs on a pinned toolchain that bashy provides itself.
3. **The agentic userland.** Shared memory, todos and sprints. Mailboxes and
   meetings between agents. A local LLM door that turns the agent CLIs you
   already pay for into an API. Tools to steer third-party agents. A
   configurable harness. A built-in container engine, local models and
   tracing. Locks on shared resources, and the many small tools agents need.
4. **sphere.** The computers you already own become one pool of local models,
   with the cloud as a thin, replaceable relay.

> bashy is to agentic tools what bash is to humans.

**Rod, not fish.** bashy ships small parts that compose, rather than a finished
workflow you have to accept whole. Anything a model may do is declared,
recorded and checked by something that is not a model.

What bashy is not:
- **Not fast.** It is about 2× slower than GNU bash on microbenchmarks.
- **Not a sandbox.** Commands run with your permissions; to contain it, put
  bashy inside a sandbox.
- **Not certified.** It is not "POSIX certified", and it is not 1.0.

## Find yourself

| you are… | what bashy gives you first | read |
|---|---|---|
| someone who writes bash scripts | the same script behaves the same on Linux, macOS and Windows, with no WSL, Git Bash or `brew install coreutils` | [§1](#1-bash-the-same-everywhere) |
| a maintainer of a repo with several languages (say Python + TypeScript) | one `dag.md` to build and smoke the repo, with pinned toolchains and no `PATH` roulette | [§2](#2-bash-one-front-door-for-a-polyglot-repo) |
| a user of **one** agentic tool who runs several sessions | sessions that don't overwrite each other's work, can message each other, and share containers, local models and secrets safely | [§3](#3-the-agentic-userland) B · F · G, [recipe A](#recipe-a-one-agent-tool-several-sessions-in-one-repo) |
| a user of **two or more** agentic tools | **bashy is the must-have**: one memory, one plan, one mailbox and one model door shared by Claude, Codex, AGY and local agents | [§3](#3-the-agentic-userland) A–G, [recipe B](#recipe-b-two-or-more-agent-tools-one-team) |
| someone with several machines, or local GPUs | your idle Macs and PCs serve local models as one pool | [§4](#4-sphere-the-computers-you-already-own) |
| a security or compliance reviewer | hash-chained audit log, dry run, declared effects, a FIPS 140-3 build target, permissive-only licensing | [§1 governance](#governance-you-can-turn-on) |

---

## 1. bash, the same everywhere

*Replaces:* a PowerShell/bash split, WSL, Git Bash, Homebrew coreutils, a
"which sed is this?" CI matrix.

> The script was fine. The sed was different.

- **One executable per platform** (shipped). Release archives are published
  for Linux, macOS and Windows on amd64 and arm64. Every release builds with
  `CGO_ENABLED=0`. The Linux build still loads glibc for its locale gate; the
  `bashy self image` artifact does not. See
  [build linkage](public-claims-evidence.md#build-linkage-and-versions).
- **A drop-in Bash 5.3** (shipped). It passes all 86 runnable fixtures of GNU
  Bash's own 5.3 test suite on Linux, macOS and Windows, with the dialect off.
  That is a fixture result, not a claim of complete interactive compatibility.
  It is drop-in on Bash's own fixtures, not "100% drop-in". See the
  [conformance statement](conformance-statement.md).
- **The userland is inside the binary** (shipped). The coreutils package
  covers the 116 POSIX utility names and the GNU coreutils programs as Go
  applets. It adds non-POSIX tools agents expect: `tar`, `jq`, `tree`,
  `which`, and a pure-Go `git`. Run `bashy commands` to see the catalog.
  Availability is not conformance; POSIX evidence is tracked per name in
  [posix-command-coverage.md](posix-command-coverage.md).
- **Windows without WSL or MSYS** (shipped). It is a native `.exe`, and it can
  rebuild itself on Windows without Git Bash, a C compiler or a preinstalled Go.
  Windows has no Unix process-group/TTY job control; see
  [job-carrier.md](job-carrier.md).
- **Small when you need small** (shipped). The lean `bash` build excludes the
  agentic layer. As v0.24.0 compressed release archives, `bash` is 5.6–6.3 MB
  and the full `bashy` is 46–50 MB
  ([claims.md](https://github.com/bashsharp/bashsharp/blob/main/docs/claims.md)).
- **Installation that works offline** (shipped). `bashy self install`
  installs `bashy`, `bash`, `sh` and a dormant `outpost` from the files
  beside it. `bashy self image` produces a `FROM scratch` container image.
  Package-manager channels (brew, scoop, winget) are **planned**.

### Governance you can turn on

> Your agent's shell should be one you can audit.

| feature | what it gives you | status |
|---|---|---|
| `BASHY_AUDIT=1` | hash-chained SHA-256 audit records naming the agent, model, session and effect class, with secrets masked in argv. `bashy inspect audit verify` checks the chain. Mapped to NIST 800-53 AU-2/3/9/10/12, SOC 2 CC7, ISO 42001 §9, EU AI Act Art. 12 ([audit-log.md](audit-log.md)) | shipped, opt-in. It records but does not block, and it cannot see past `execve` or into builtins |
| `--dry-run` | runs the script's logic and prints the external commands instead of running them. Before you pipe curl into bash, pipe it into `bashy --dry-run` ([dryrun.md](dryrun.md)) | shipped |
| declared effects | `@guard(effects: …)` caps what a function may do; `destroy`, `spend`, `cred` and `priv` require confirmation; unclassified commands fail closed ([effect-derived-confirmation.md](effect-derived-confirmation.md)) | shipped (Bash#, alpha) |
| `bashy approve` | one queue of decisions waiting for a human: a yielded `@confirm`, an ACP permission request, a chat grant | shipped |
| `bashy otel` | bounded, agent-readable answers from your own traces: `cost`, `failed`, `why-slow` ([observability.md](observability.md)) | shipped. OTel export of audit records is planned |
| FIPS 140-3 | `make build-fips` builds against the Go Cryptographic Module (CMVP #5247), and still passes the 86 fixtures ([fips-140.md](fips-140.md)) | build target. No FIPS release asset yet |
| supply chain | only MIT/BSD/Apache code is compiled in. Tools bashy downloads are pinned and checksum-verified, and fail closed ([licensing-supply-chain-policy.md](licensing-supply-chain-policy.md)) | shipped |

---

## 2. Bash#: one front door for a polyglot repo

*Replaces:* a Makefile that calls `npm`, `uv`, `cargo` and `go` with four
quoting rules, four `PATH`s and a README section titled "Prerequisites".

The full language case is in
[Why Bash#](https://github.com/bashsharp/bashsharp/blob/main/docs/why-bashsharp.md).
For a repository it comes down to three shipped pieces:

- **`dag.md` is the build file** (shipped). Each `### target` heading is a
  task. Tasks have `Requires:` / `Sources:` / `Generates:` lines,
  content-hashed up-to-date checks, `-j`, `--json`, `--explain`, and `--mesh`
  to run a body on another host. `Effects:` is recorded but not yet enforced
  (**planned**); see [dag.md](dag.md).
- **Several languages in one file** (shipped). A `~~~py`, `~~~ts`, `~~~rs`,
  `~~~c`/`~~~cxx`, `~~~go`, `~~~csharp` or `~~~powershell` block becomes a
  callable, and typed values cross the boundary. Each block runs as a real
  process in the project's own environment: the nearest `.venv`, or
  `package.json` plus its lockfile. Your manifests and lockfiles stay
  authoritative. Bash# does not reimplement uv, npm, pnpm or cargo.
- **Toolchains that never come from `PATH`** (shipped). Go, `zig cc`, uv with
  CPython, Node with TypeScript, Bun, rustup and PowerShell 7 are pinned
  downloads, verified by checksum. A script means the same thing whatever
  happens to be installed. `bashy check --prepare` fetches them ahead of time,
  for example into a container image.

Proof on real code: [`examples/dag/`](../examples/dag/) drives unchanged
checkouts of upstream projects through their own build and smoke targets.
- **Python:** nanochat, mini-swe-agent
- **TypeScript:** opencode, openclaw
- **Rust:** uv, codex, bun, mise
- **C/C++:** ffmpeg, curl, git, llama.cpp, tesseract, cmake
- **Go:** gh, hugo, caddy

The **hermes-agent** example mixes Python and TypeScript in one body. A gate
checks that `git status` is byte-identical before and after each run.

Planned: lists and dicts as values on both sides of a fence, an island used as
a pipeline filter, and the "at home" work for Python and TypeScript users
(PEP 723, `uvx`, tsconfig, JSX/TSX fences).

---

## 3. The agentic userland

The agentic userland is called Yoke. *bashy is the shell; Yoke is the harness
that puts agents to work in it.* Every verb below is a plain command with
`--help` and usually `--json`. That makes it equally usable by a person, a
script, or any agent CLI that can run a shell command. `bashy install-agent`
makes bashy the shell, or the MCP server, for Claude Code, Codex, OpenCode,
Aider and others ([mcp-server.md](mcp-server.md)).

### A. Shared context: one memory and one plan, whichever tool is typing

| verb | what it does | status |
|---|---|---|
| `bashy kb` | agent memory as a wiki of small Markdown pages. Inside a repo it lives in `docs/kb/` and travels with the clone; there are also host and per-agent rings. `kb context --budget N` builds a size-limited context for a task. `kb sources` finds each vendor's private memory store so you can move it into one shared memory | shipped |
| `bashy todo` | a task list committed in `docs/todo/`, with a personal list outside repos. Every tool reads the same list | shipped |
| `bashy sprint` | a board that spans repos: goals, stories, a manager lease, claim → submit → accept. Agents from different vendors work one plan with one named owner ([sprint lifecycle](https://github.com/qiangli/yoke/blob/main/docs/sprint-lifecycle.md)) | shipped |
| `bashy craft` | a skill graph learned from real runs: which skills actually worked here | shipped |
| `bashy handoff` / `resume` | pause a session and continue it in another tool or on another machine | shipped |

### B. Communication: agents that can actually reach each other

| verb | what it does | status |
|---|---|---|
| `bashy inbox` | one view over every inbound source. It reads without consuming and tracks what you have seen ([unified-inbox.md](unified-inbox.md)) | shipped |
| `bashy mb` | the host message board, append-only. A post to an offline agent waits for it. Address a recipient by name, band, tool or role (`--role conductor`) | shipped |
| `bashy notify`, `ping` | one-line notifications to an agent or role | shipped |
| `bashy bus`, `activity` | a change feed: "kb holds facts; the bus carries changes". Subscribe to what you care about | shipped |
| `bashy meet` | turn-taking meetings across agent CLIs and a human. A note-taking secretary files the minutes. `meet consult` convenes a one-shot cross-vendor panel. `meet serve` opens it to a browser or phone, and `meet dm` lets a paired messenger drive an agent | shipped |

### C. The LLM door: the subscriptions you already pay for, as an API

`bashy llm` is the host's single door to model capacity. It is one local
endpoint on port 24556 that speaks the **OpenAI, Anthropic and Ollama**
protocols. Behind it sit two kinds of backend:
- bashy's own local models;
- **cligw**, which serves the agent CLIs you already subscribe to (claude,
  codex, agy, …) as a pool of pre-started, one-shot workers. Each worker gets
  exactly one prompt and is retired, so no context leaks between requests. The
  CLI's own tools are off. Client `tools` are emulated and returned as
  ordinary `tool_calls`.

A request names a band (`L4`, `L4+`, `auto`), a model or an agent. **Sticky
bindings** freeze the exact model or CLI version for N requests, so a
benchmark compares harnesses rather than routing luck.
`bashy llm env --client …` prints the variables a client needs; for recipes,
see [llm-client-recipes](https://github.com/qiangli/yoke/blob/main/docs/llm-client-recipes.md).
Status: shipped. Authentication is an owner token or an owner-only unix socket.

bashy provides the mechanism. Whether your vendor's terms allow a given use is
for you to check.

### D. Steering third-party agents

| verb | what it does | status |
|---|---|---|
| `bashy weave` | run several agent CLIs in parallel on one repo. Each issue gets its own git-clone workspace. You can `say`, `pause`, `salvage`, `review` and `pull` the result back; `autopilot` loops claim → launch → verify → merge | shipped |
| `bashy chat` | send one governed instruction or open a live session, routed by `--agent`, `--band` or `--capability`. `chat sessions` lists every live agent; `attach`, `steer`, `interrupt` and `grant` act on one remotely ([one-agent-control.md](one-agent-control.md)) | shipped |
| `bashy foreman`, `delegate` | a persistent session you can steer, or a task handed to another agent (or a detached copy of yourself) | shipped |
| `bashy coach` | an auto-coach with no LLM that ESCs an agent out of a doomed tool loop | shipped |
| `bashy agent clone` | a second agent that starts where another one is now. It reports whether the context really branched | shipped |
| `bashy leaderboard`, `stats` | rank your agents on runs that actually happened here, with confidence intervals and cost per solve. Bands L1–L5 decide who may code, manage or judge | shipped |

### E. genie and ycode: a harness you configure instead of one you rent

- **`bashy ycode`** (shipped) is the engine for agents declared in YAML. An
  `agent.yaml` becomes a harness with sessions, plans, memory, skills,
  `serve`, `acp` and `repl`. You can change the prompt, tools and loop without
  forking anyone's CLI.
- **`bashy genie`** (shipped) is a vendor-free SWE agent on that engine. It
  runs on local models through the LLM door by default, and `-m` picks any
  model the door serves. Forms: a one-shot `genie "…"`, `genie solve` for
  patch-producing runs with a run record, and `genie web`. `--yolo` selects
  the authored unattended profile.

### F. Built in: the O3 pillars (ollama · oci · otel)

| verb | what it does | status |
|---|---|---|
| `bashy ollama` | a managed local LLM runtime with its own daemon, port and models. The door runs it as one exclusive device with a priority queue: interrupt > steering > interactive > batch | shipped (macOS, Linux) |
| `bashy oci` (`sandbox`) | an embedded container engine with a podman-compatible CLI: build, run, kube, compose. No Docker Desktop and no daemon; on macOS and Windows it uses a Linux machine VM | shipped |
| `bashy otel` | query your own traces: `cost`, `failed`, `why-slow`, `guessed` | shipped |
| `bashy limit` | k8s-style memory, pids and disk limits on a whole process tree | shipped |
| `bashy service`, `supervisord` | declare services as YAML units; a supervisor keeps them healthy with health checks, restart backoff, live reload and `--pid1` | shipped |

### G. The other tools agents need

| verb | what it does | status |
|---|---|---|
| `bashy resource` + `bashy claim` | **leases on shared things.** `claim` with no argument holds the current project (the repo plus its sibling repos). `bashy resource add` names anything agents must not use at the same time, such as a test host, a GPU, a model or a port. `claim NAME -- CMD` holds it for the command's lifetime; `claim release` hands it back. A conflict exits 9 and names the holder | shipped |
| `bashy ask` | asks the **human**, never the model, for a one-off value such as a token or an OTP. It returns a 0600 file path, never the plaintext | shipped |
| `bashy secret` | vault-backed API keys. `eval "$(bashy secret env)"` replaces plaintext rc files. Redaction is guaranteed only for vault-issued secrets | shipped |
| `bashy dag` | make for agents; see §2 | shipped |
| `bashy search`, `fetch` | indexed file search, and web search with citations | shipped |
| `bashy inspect context` | one first call that tells an agent where it is and what it can do | shipped |
| `bashy commands add` | register your own commands so every agent can discover them | shipped |

---

## Recipe A: one agent tool, several sessions in one repo

Most people run several sessions of the same CLI side by side, and the
sessions cannot see each other. That is how one session sweeps another's
staged files into its commit. In these examples, `do1`, `gpu0` and `stage`
name things your sessions share; `<who>` is another session's name. You
need three things.

**1. Leases, the must-have.** A lease stops two sessions from colliding.
Messages alone do not: two agents chatting politely still overwrite each
other's git index.

```bash
bashy claim --intent "refactor parser"          # hold this project while you write
bashy claim do1 -- make test                     # hold the shared test host for the test's lifetime
bashy resource add stage --kind path /w/stage    # register a shared thing once...
bashy claim stage --wait 10m                     # ...then claim it by name, waiting for the holder
bashy claim list                                 # who holds what, right now
bashy claim release stage                        # hand it back
```

**2. A way to talk** (B):

```bash
bashy mb send <who> "parser refactor pushed; rebase before touching lexer.go"
bashy inbox                                      # what reached me, from any source
bashy chat sessions                              # every live session on this host
```

**3. Shared, safe infrastructure** (F, G):

```bash
bashy oci run --rm -v "$PWD":/w alpine sh -c 'cd /w && ./smoke.sh'   # isolate the risky part
bashy claim gpu0 -- bashy genie "summarize the failing tests"         # one local model, one queue
bashy ask --name STAGING_OTP --stdout | bashy secret set staging-otp # the human types it
```

## Recipe B: two or more agent tools, one team

With several vendors, each CLI keeps its own private memory, plan and
conventions. bashy is the common floor beneath them, which is why it is a
must-have here:

```bash
bashy kb sources                                 # find each vendor's private memory
bashy kb context --for "release plan" --budget 4000   # one shared, size-limited context
bashy todo add "port lexer to new AST" --owner <codex-agent>
bashy meet consult --topic "Is the AST change safe to merge?"   # cross-vendor panel, minutes filed
bashy weave add …; bashy weave start …           # fan issues out to isolated workspaces
eval "$(bashy llm env)"                          # every tool and script on one model door
```

The usual flow: Claude writes the plan into `kb` and `todo`. Codex picks up
the story in a `weave` workspace. A third vendor reviews it in `meet`. genie
runs the cheap mechanical steps on local models through the same door. And
`sprint` records who owns what, while `claim` stops any two of them writing
the same files.

---

## 4. sphere: the computers you already own

*Vision:* the computers you already own become one coherent, self-hostable
mesh that agents can operate. It serves pooled LLMs on your own models, and
the cloud is only a thin, replaceable relay.

- **Pairing** (shipped). `bashy login` / `bashy tessaro` pair a machine with
  the Tessaro portal ([tessaro.sh](https://tessaro.sh)) so it can join a
  sphere.
- **A pooled endpoint** (shipped). Each paired machine reports its local
  models. You get one OpenAI-compatible `/v1` endpoint for models, chat and
  embeddings. It routes to the least-loaded machine, supports streaming,
  audits every dispatch, and is shared under a pool access list.
- **The front door** (shipped). `bashy peer` (`bashy sphere`) covers `mesh`,
  `pool`, `peers`, `shard` and `status`. Machines on the same LAN link
  directly.
- **Sharding** (prototype). Splitting one model's layers across machines has
  been demonstrated across two ranks.
- **Planned:** sharding as a one-click option for large models, and
  cross-LAN hole punching.
- **Beyond sphere.** `bashy kubectl` and `bashy helm` (shipped) manage a
  self-hosted Kubernetes cluster as provisioned, pinned tools.

---

## Compared to

| instead of… | bashy wins on | bashy loses on |
|---|---|---|
| Git Bash / WSL | a native `.exe`, Bash 5.3 + coreutils in one file, no distro to maintain | WSL runs real Linux binaries; bashy runs scripts |
| stock bash + Homebrew coreutils | the same tools and flags on every OS, an audit log, dry run | GNU bash is about 2× faster and preinstalled on most Linux |
| tmux + ad-hoc scripts for several agents | leases, mailboxes, a shared plan and steering, with `--json` | tmux is universal and needs no adoption |
| a sprawl of MCP servers | one binary, one `--help` and `--json` contract, also exposed as an MCP server | a specialized MCP server can go deeper in its niche |
| LiteLLM / OpenRouter | local, owner-only, serves the CLI subscriptions you already pay for, with sticky bindings for fair benchmarks | hosted routers reach more providers and need nothing on your machine |
| plain Ollama | an exclusive-device queue shared by every shell and agent, plus pooling across your machines | plain Ollama is simpler when one person uses one model |
| Docker Desktop | an embedded, daemonless podman-compatible engine with no licence seat | Docker Desktop's GUI and ecosystem integration |
| make / just | `dag.md` with content hashing, pinned toolchains, `Require:`/`Ensure:` | while `Effects:` is unenforced, `just` is the honest pick for plain task running; make is everywhere |
| CrewAI / LangGraph | orchestrates the agent CLIs you already use, from the shell; no SDK to code against | an SDK is the better fit for an agent product you ship |

## When not to use it

- **Throughput is the constraint.** GNU bash is about 2× faster.
- **You need a sandbox.** bashy runs with your permissions; put it inside one.
- **You need Windows administration cmdlets.** Use PowerShell.
- **You need hermetic builds.** Use Nix.
- **You need a stable 1.0 today.** bashy is alpha; Bash# may still change
  through RFCs before 1.0.

## Status by layer

| layer | shipped | prototype | planned |
|---|---|---|---|
| bash | Bash 5.3 (86/86 fixtures on three OSes), Go coreutils, cgo-free releases, self install/image, audit, dry run, otel | | package-manager channels, FIPS release asset, audit export to OTel |
| Bash# | `dag.md`, nine fence languages, pinned toolchains, `check --prepare` | | `Effects:` enforcement, list/dict values, Python/TS "at home" |
| agentic userland | kb, todo, sprint, craft, inbox, mb, bus, meet, llm door, weave, chat, agent clone, leaderboard, ycode, genie, ollama, oci, otel, limit, service, supervisord, claim, resource, ask, secret, approve | | live session roster, generalized attach |
| sphere | pairing, pooled `/v1`, `peer`, LAN links, kubectl/helm | layer sharding | one-click shard, cross-LAN hole punching |

## Where the numbers come from

- [public-claims-evidence.md](public-claims-evidence.md): build linkage, job
  control, command counts, and how to recheck each item on a release
  candidate.
- [claims.md](https://github.com/bashsharp/bashsharp/blob/main/docs/claims.md):
  every Bash# and fixture figure, with its corpus, revision and host.

If a sentence here carries a number that is in neither file, the sentence is
wrong.
