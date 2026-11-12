# bashy overview

What bashy is, layer by layer and tier by tier: the long form of the [README](../README.md)'s *At a glance*.

- **bash + coreutils in pure Go**: one executable per platform, no system bash,
  no GNU-vs-BSD drift.
- **GNU Bash 5.3 compatible**: Bash's own 5.3 suite, 86/86.
- **Built to POSIX.1-2016 (IEEE Std 1003.1-2016)**: 493/493 on the licensed
  shell-isolation arm; the full Shell and Utilities run retains 67 non-passes.
  [Measured status and certification follow-up](vsc-pcts-run-status.md).
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

Deciding whether to use it, or which part of it is for you: [docs/why-bashy.md](why-bashy.md).

The shell and userland are Go. Release builds use `CGO_ENABLED=0` on Linux,
macOS and Windows; inherited ignored signals are preserved by bashy's product
runtime build, not by C. The Linux `bashy` still loads glibc dynamically for its
locale gate. See [build evidence](public-claims-evidence.md).

## Software Shaping: make your tools fit you

Describe a change to an app you use — in plain words — and get a candidate you
can review, verify and approve for yourself, with history and undo: your
version of the tool, and a way back. Web shaping is an **opt-in trial**:
personal versions affect the selected user only. bashy itself is shapeable
this way. **[Start with Hello World →](../examples/shaping/hello-world/README.md)**
The canonical definition, rules and status are in
[software-shaping.md](software-shaping.md).

### The promotion path

A shaped change moves local host -> team/org -> upstream bashy, and new
features flow back the other way:

```bash
bashy shape wish bashy "make lexicon accept JSON lines"  # candidate
bashy shape verify bashy <id>                            # trusted verifier
bashy shape approve bashy <id>                           # activate for me
bashy shape undo bashy                                   # way back

bashy shape remote set org <url>     # name the team/org remote
bashy shape outgoing --to org        # exit 3: something is worth promoting
bashy shape push --to org            # approved change -> team/org
bashy shape push --to upstream --pr  # propose it upstream (prints compare URL)
bashy shape incoming --from upstream # exit 3: upstream has new work
bashy shape pull --from org <id>     # gated wish -> verify -> approve; undoable
```

Every verb takes `--json` and uses agent-friendly exit codes — 0 nothing
pending, 3 pending, 1 error — so an agent can drive the whole path without
parsing prose. See the [web trial guide](shaping-trial.md).

**Status:** the `bashy shape` CLI lifecycle
(wish/verify/preview/approve/undo/restore/submit) and the promotion rod
(remote/outgoing/push/incoming/pull) ship in v1.0.0-rc.27. Cross-platform
shaping validation is still in progress (Sprint 579), as is the team
adoption policy; open items are tracked in the
[known-issues ledger](releases/v1.0.0-rc.27-known-issues.md).

## Ten minutes

Install bashy using one of the methods in [Install bashy](#install-bashy), then
check it with `bashy --version`.

Then take the tour — 27 small programs with pinned transcripts and one
script that runs them all on your machine, also written as a procedure your
coding agent can drive: **[bashsharp/tour](https://github.com/bashsharp/tour)**.
The ten-minute version is in this repo: [`examples/quickstart/`](../examples/quickstart/).

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
  For yash, see [yash-chunks.json](../yash-chunks.json) and the
  [measurement boundaries](public-claims-evidence.md); fixture completion
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
| **sphere** | a peer-to-peer mesh formed by the outposts on the machines you own: they share capabilities with each other directly. For LLMs: **pool** (the machines' GPUs and local models serve as one endpoint) and **shard** (a model too large for any one machine runs split across several) | `bashy sphere` |
| **cluster** | a standard Kubernetes cluster (DKS) whose nodes are those same outposts, plus a satellite as control plane; run ordinary Kubernetes workloads on your own hardware | `bashy kubectl`, `helm` |
| **cloud** | external providers | `bashy doctl`, `aws`, `gcloud` |

**The workplace: agents managing agents.** In bashy an **agent** is a
binding of a tool to a model, written `tool:model` (for example
`codex:gpt-5`). Supported out of the box:

- **claude**, **codex**, **agy**, **muse** and **opencode**, the agentic
  coding CLIs you already use;
- **genie**, bashy's own built-in agent, with `bashy ycode` as its TUI and web chat.

Any of them can manage the others with the same bashy tools:

- `bashy sprint` plans and tracks the work;
- `bashy weave` runs each worker in its own isolated git workspace, then
  verifies and merges the result;
- `bashy foreman` starts, steers and watches a long-running agent;
- they talk to each other with the same tools:
  - `bashy mb` (message board) to send;
  - `bashy inbox` to read;
  - `bashy meet` for live rooms, humans included;
  - `bashy kb` for shared knowledge;
  - `bashy todo` and `bashy sprint` to hand work over with its context.

A conductor on one vendor can staff workers on the others and review what they
deliver. That spreads load across subscriptions and models while every agent
uses the same shell and userland.

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
[docs/internal/campaign-and-gates.md](internal/campaign-and-gates.md).

Python packages used across the repos, with CPython 3.15 wheel readiness:
[docs/python-package-inventory.md](python-package-inventory.md). Refresh it with
`DHNT_ROOT=<umbrella checkout> bashy python scripts/python-package-inventory.py`.
