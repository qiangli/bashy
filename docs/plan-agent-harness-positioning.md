# bashy vs. an agent harness — gap analysis and positioning plan

**Status:** analysis + plan. This document records design; it does not implement it.
**Reference system:** DeepSeek Harness (`dsh`), MIT, developer preview, studied at
`0.1.0-rc.8` (226 workspace packages, 58 `ctx.*` services, 26 tool packages, 63
model-facing tools).
**Question asked:** bashy's agentic surface looks aligned with `dsh` — should bashy
*be* an agent harness, with every command a "plugin" and bash itself the plugin
system?

---

## 0. The answer, up front

**Revision, 2026-08-19.** The first version of this document answered "no — bashy should
not be an agent harness." That answer was reached without opening the Bash++ design docs,
and Bash++ is the load-bearing part of the argument. **The conclusion moves.** What
follows is the corrected position; §5.4 states exactly what changed and why.

**bashy should be an agent harness — a harness *kit*, not a harness *product*. The loop
should be authorable in Bash++, never compiled into Go.**

The distinction is not a hedge, and it is not mine — it is `dsh`'s own deepest claim,
restated in the shell idiom:

> *"There is no privileged core to patch: you extend dsh by mounting a plugin beside the
> others."* — `dsh`, `docs/architecture.md`

`dsh` means it: even `ctx.agentLoop` is a replaceable service. **bashy can make that
literally truer than `dsh` does** if P3 makes an executable the plugin format — any
language already on the machine, no privileged runtime to be inside — while its
composition language remains a shell. Where `dsh` says "the loop is a plugin", bashy can say
**"the loop is a script"**, which is the same sentence with a lower floor.

So the two positions reconcile cleanly:

| | verdict |
|---|---|
| A privileged agent loop compiled into Go, shipped as bashy's opinion of how agents work | **No.** §5.1 — competes with the fleet it conducts, breaks the enforced local-first claim, costs the shell. |
| The primitives that let *anyone* author a loop, in a language bashy already owns | **Yes.** That is the substrate move, not the harness move, and it is roughly two orders of magnitude less work. |

**One loop-specific runtime primitive is missing, and it is small; the planned authoring
language and several substrate hardening phases are still unbuilt.** Most reusable
ingredients already exist — 292 effect-typed tools, a shell dispatch pipeline, a fleet
plane, skills, knowledge, a DAG runner, and an audit chain. What the loop itself lacks is
a command that turns
*messages + tool schemas* into *an assistant message + tool calls*. Call it `bashy llm`.
With it, a harness works today as a few hundred lines of plain bash + `jq`; P6 makes that
loop pleasant in Bash++. **Everything else in bashy is already the harness.** Without the
verb, even a finished Bash++ would only be a better language for scripts that call other
people's harnesses.

**The acronym is earned, with one condition.** BASHY — **B**ashy's **A**gentic **S**hell
**H**arness **Y**oke — is recursive in the GNU lineage, and *yoke* is doing real work
rather than filling a letter: the crosspiece that couples two draft animals to one load
(the agent and the machine, pulling together), and the control yoke you steer with. It is
compatible with the positioning decision of record — "the local-first runtime your coding
agents run in" is the *sentence*; the yoke is the *picture* of what that sentence
describes. The condition is §5.5: it names an architecture, not a shipped capability, and
publishing it before the primitive exists would put bashy in the red column of its own
honest-claim ledger.

**The expansion names layer 3, not the product — the two nest.** Read the phrase
grammatically: *Bashy's Agentic Shell Harness Yoke* has head noun **Yoke**, modified by
*Agentic Shell Harness*, possessed by *Bashy*. It therefore names **a yoke belonging to
Bashy**, which is exactly the third substrate (`../CLAUDE.md` §Name,
`../../docs/bashy-yoke-framework.md`) — not the whole. So the acronym names the product and
its expansion names the product's top layer; that nesting *is* the recursion, and it is
why **Yoke** may be used unqualified as the layer-3 name without colliding with BASHY the
product name.

## 1. What `dsh` actually is

`dsh` is built on Cordis, a plugin framework where a running application is a tree of
plugins contributing services, typed events and *reversible* effects to a shared
context. Its own words: *"There is no privileged core to patch: you extend dsh by
mounting a plugin beside the others, and registrations are effects that unwind when
their plugin unloads."*

Four mechanisms carry the whole design:

1. **Capability seam** — a swappable capability is always three roles: a **Service
   Definition** (owns `ctx.<key>` and the vocabulary), one or more **Service
   Providers**, and one or more **Consumers** (usually a model-facing tool). `packages/shell`
   is the canonical example: `dsh-shell` defines `ctx.shell`, `dsh-bash-local` /
   `dsh-bash-sandbox` provide it, `dsh-tool-bash` consumes it. A seam is the complete
   triple; one role alone is not a seam.
2. **The session log is the truth.** `deriveMessages()` projects model history from an
   append-only `SessionEvent` stream. The invariant is enforced, not documented:
   **model-visible ⟺ logged**. A new model-visible input *requires* a new session
   event, and a runtime invariant asserts it.
3. **Composition is data.** A running `dsh` is a plugin tree assembled at boot from
   ordered layers: bundles (npm packages declaring `dsh.bundle`) → profile patch → home
   patch → `--patch` overlay. `dsh --profile web --dump-config` prints the tree; any row
   it prints can be replaced by a patch. Out-of-tree plugins install with
   `dsh plugin --profile <name> add <package>`; third-party plugins are discoverable by
   a GitHub topic.
4. **The pipeline is where policy lives.** Tool calls pass a documented gauntlet:
   `tools/pre-execute` (hooks, permission, sandbox — may **deny** or **ask**) →
   registered monotonic guards → `ctx.approval` one-shot prompt → `tools/execute`
   (around-dispatch: timeout, retry, metrics) → the tool body → `fs/*` intent gates →
   `tools/post-execute` (accept / block / **replace** / add context) → registry
   normalization → `finalizeContent` → `tools/result`. None of that requires changing
   the loop.

Everything else is instantiation: `ctx.llm`, `ctx.fs`, `ctx.subprocess`, `ctx.sandbox`,
`ctx.terminals`, `ctx.jobs`, `ctx.skills`, `ctx.subagents`, `ctx.web`, `ctx.lsp`,
`ctx.spillStore`, `ctx.compaction`, `ctx.goals`, `ctx.workflowEngine`, `ctx.approval`,
`ctx.sessionQuery`, and ~40 more.

**Its shell is one seam out of 58.** `dsh-bash-local` spawns `bash -c <command>` per
call, and its own README lists the limitations:

- *"Unconfined by itself"* — needs `dsh-bash-sandbox` to confine.
- *"No persistent shell or PTY — every call starts a fresh non-login `bash -c`."*
- *"POSIX-only — the `bash` binary is hardcoded... Windows is unsupported."*

That list is worth re-reading, because it is close to a description of bashy's feature
set.

---

## 2. What bashy actually is (measured this session, not quoted)

| Measurement | Value | How |
|---|---|---|
| Commands in the Command Atlas | **292** | `bashy commands --atlas` |
| Agentic verbs (orchestration/knowledge/code-intel/platform/diagnostics) | **53** | atlas group view |
| Top-level dispatch clauses | **76**, one Go `switch` | `agentos.go` |
| Native LLM chat client with tool-calling | **none** | no `chat/completions`, no `tool_calls`, no provider SDK anywhere in the tree |
| Turn coverage bashy claims for itself | `subprocess_harness_turn`, `CoverageComplete: false` | `pkg/chat/genai.go` |
| ACP direction | **client only** — launches an agent subprocess | `pkg/acp/acp.go` |
| A2A direction | **client only** — dials a peer | `pkg/herald` |
| In-process confinement (Landlock/Seatbelt/bwrap) | **none** | only vendored podman seccomp |
| Out-of-tree verb installation | **none** — no `bashy plugin` | atlas |

The single most important line there is the fourth one. **There is no LLM loop in bashy
or coreutils.** `pkg/ollm` wraps Ollama for *model management* (list/pull). `pkg/chat`
is a *subprocess harness driver* — it launches `claude`, `codex`, `opencode`, `ycode`,
`agy` through `agentpty` and reads their output. Its own telemetry says so, in a
constant: the turn it observes is a `subprocess_harness_turn`, and it declares its
coverage incomplete.

This is not a deficiency to hide. It is the architecture, and `docs/philosophy.md` §3
already states it plainly: *"The one thing bashy cannot compute for itself: inference."*

What bashy *does* own, and `dsh` does not:

- **A real shell.** Bash 5.3 drop-in at 86/86 on Bash's own suite, a POSIX certification
  campaign, pure Go, `CGO_ENABLED=0` to six platforms including Windows.
- **A real userland.** ~150 pure-Go coreutils/textutils/fileutils running *in process*,
  cross-platform, no fork.
- **A guarded execution pipeline at the shell seam** (`agentos.go:1562–1618`), composed
  outermost-first: `telemetry → audit → execlog → advisor → learn → weaveguard →
  dry-run → coreutils userland`. Every external command a harness asks the bashy
  interpreter to resolve passes through it. Front-door `bashy <verb>` dispatch currently
  happens before this chain; P0 must unify those paths rather than claiming coverage it
  does not have.
- **A fleet plane** far richer than `dsh`'s subagent family: `weave`, `foreman`, `meet`,
  `sprint`, `judge`, `delegate`, `supervise`, `coach`, `pair`, `relay`, `bus`, `board`,
  `room`, `claim`, `handoff`, `herald`, plus bands/nicknames/capability routing.
- **A knowledge plane**: `kb` (BM25), `craft` (capability keys, fact/fold), `graph`
  (code + repo-knowledge + execution), `recall`, `lexicon`, `skills`.
- **An agent-facing envelope surface**: `context --json`, `run`, `check`, `--dry-run`,
  `verify`, `doctor`, `commands --atlas`. This is bashy's structural mirror of `dsh`'s
  prompt-and-tool-schema assembly — the same information, delivered to a harness bashy
  does not control, over a CLI instead of an in-process registry.

---

## 3. The inversion

```
dsh                                  bashy
─────────────────────────────        ─────────────────────────────
  agent loop        (owned)            agent loop        (delegated to a CLI)
   └ ctx.tools      (registry)          └ Command Atlas   (292 verbs)
      └ ctx.shell   (one seam)             └ bash 5.3     (the whole product)
         └ bash -c  (a plugin)                └ harness    (a subprocess)
```

`dsh` mounts bash **inside** the harness. bashy runs the harness **inside** bash.

Both are coherent. The difference that matters strategically: `dsh`'s position requires
it to win the harness market to matter. bashy's position pays off whenever *anyone's*
harness runs — including `dsh`'s. There are perhaps a dozen serious harnesses and one
shell interface they all speak.

---

## 4. Side-by-side gap matrix

Verdicts: **AHEAD** (bashy stronger) · **PARITY** · **GAP** (bashy weaker, matters) ·
**N/A** (only meaningful if bashy owned a loop).

| # | Capability | `dsh` | bashy | Verdict |
|---|---|---|---|---|
| 1 | Agent loop (turn/step) | `ctx.agentLoop`, waterfall extension points | none — delegates to a harness subprocess | **N/A as Go code; becomes AUTHORABLE via P0.5 + P6** |
| 2 | LLM adapter seam | `ctx.llm`; DeepSeek, pi-ai, replay providers | none native; fleet catalog + pooled gateway; Ollama mgmt only | **GAP — one verb (P0.5). Revised from N/A; see §5.5** |
| 3 | Prompt assembly | `ctx.systemPrompt`, sections + tool schemas | `context --json`, `commands --atlas`, skills — delivered to a foreign harness | **PARITY, different plane** |
| 4 | Durable session log | append-only `SessionEvent`; model-visible ⟺ logged; JSONL + SQLite; fork/resume | `execlog` records **argv/exit only, never stdout**; harness transcripts are the only reasoning record and expire | **GAP — the deepest one** |
| 5 | Session search / trace | `session_search`, `session_trace`, `session_event_*`, SQLite FTS | `kb search` (BM25) over curated pages; no transcript search | **GAP** |
| 6 | Guarded tool pipeline | pre/execute/post waterfalls + monotonic guards, registry | ExecHandler middleware chain for shell-resolved externals; front-door verbs bypass it; **compile-time `append`** | **PARITY in shape, GAP in coverage and openness** |
| 7 | Deny / ask on a call | `tools/pre-execute` denies; `ctx.approval` asks; permission presets | **nothing can deny.** audit records, advisor advises, weaveguard warns | **GAP — highest value/effort ratio** |
| 8 | Process confinement | `ctx.sandbox`: bwrap → Landlock → Seatbelt → Windows ACL, per call, fail-closed | container-level only (`podman`), plus `weave` git isolation | **GAP** |
| 9 | Filesystem policy | `fs/*` gate: read-before-edit, version-guarded writes, observed state | real userland commands; **no stale-write guard** | **GAP (narrow, real)** |
| 10 | Output bounding / spill | `ctx.spillStore`: per-stream caps, `lossy` flag, spill files | `run --capture` unbounded; no spill | **GAP (cheap to close)** |
| 11 | Background jobs | `ctx.jobs` + `job_list/output/kill` | POSIX job control + `pkg/jobs` + `disown/nohup/setsid` | **AHEAD** |
| 12 | Persistent PTY | `ctx.terminals` + 6 model tools | `agentpty` + live-session socket — used to drive *agents*, not exposed as a general tool | **PARITY, unexposed** |
| 13 | Subagents / delegation | `ctx.subagents` × 7 providers, control tools, Agent Teams, Ralph | 15+ verbs, cross-harness, banded, capability-routed, gate-settled | **AHEAD** |
| 14 | Skills | `ctx.skills` + filesystem provider + loader tool | same model, plus `craft` compose-on-demand and attested `skills run` | **AHEAD** |
| 15 | Knowledge / memory | session-query FTS, session-reference | `kb`, `craft`, `graph`, `recall`, `lexicon` | **AHEAD in breadth** |
| 16 | Context compaction | `ctx.compaction`, token meter, tool-result pruner | none | **N/A** |
| 17 | Hook bridges | *consumes* Claude Code + Codex hook protocols | `install-agent` forces the shell — the inverse direction only | **GAP (high leverage)** |
| 18 | MCP | `mcp-client` registers external MCP tools | none | **GAP (arguable)** |
| 19 | Be-an-agent protocol server | ACP server, SDK JSON-RPC server, TS + Python clients | **client only** — cannot be driven | **GAP** |
| 20 | Plugin system | Cordis: bundles, profiles, patches, `plugin add`, reversible effects | **76 hardcoded `switch` clauses**; extension = skills / dag / PATH | **GAP — the headline** |
| 21 | Runtime self-modification | `cordis_define` / `cordis_run` — the agent writes and mounts a live plugin | `craft compose`, `dag`, `skills` — prompt/script level | **GAP (interesting, not urgent)** |
| 22 | Human UI | Web app + TUI + conversation nodes + settings cards | `--web-ui` designed, not built | **GAP** |
| 23 | Config as data | `cordis.yml` + layered patches + generated config catalog | env vars + per-kind YAML under a config dir | **PARTIAL** |
| 24 | Observability | `ctx.sessionTelemetry` → OTel | OTel exec middleware + GenAI spans + hash-chained audit + FIPS mode | **AHEAD** |
| 25 | Shell fidelity | `bash -c` per call; no state; POSIX-only; Windows unsupported | Bash 5.3 86/86; POSIX cert campaign; pure Go; 6 platforms incl. Windows | **AHEAD, decisively** |

Score, for what a score is worth: **6 ahead, 2 parity, 14 gaps, 2 not-applicable, and
1 partial.**
The gaps cluster in three places — *openness* (rows 6, 17, 19, 20, 21), *governance of a
single command* (rows 7, 8, 9, 10), and **one missing primitive** (row 2). Nothing in the
gap column requires bashy to own an agent loop; row 2 requires only that it be able to
*place a model call*, which is a different and much smaller thing. That is the finding.

**Revision note:** row 2 was scored **N/A (deliberate)** in the first version, on the
reasoning that inference is external so the seam is out of scope. That conflated the
*seam* with the *service*. bashy already declares `net,spend` verbs that reach a model
through someone else's CLI; refusing to reach one directly is not a principle, it is a
missing verb. §5.5 corrects it.

---

## 5. The strategic question, answered

### 5.1 Why bashy should not become a harness *product*

This subsection argues against ONE thing: a privileged agent loop compiled into Go and
shipped as bashy's opinion of how an agent works. It is not an argument against §5.4.

1. **The loop needs a model, and `philosophy.md` §3 already says inference is the one
   thing bashy cannot compute by itself.** A hosted-model loop uses the network; the T0
   Ollama provider gives the same seam a local, air-gapped answer. That makes inference
   available, but it does not justify compiling one privileged loop into bashy: provider
   access and loop policy are separate concerns.
2. **The moat is the shell, and the shell is not close to finished.** Bash 5.3 is 86/86
   but POSIX certification is an open campaign with a live failure queue. Every engineer-week
   spent on a chat loop is a week not spent on the one asset nobody else in this space
   has and that takes years to acquire.
3. **The market is saturated at the loop and empty at the substrate.** Claude Code,
   Codex, `dsh`, opencode, aider, and a first-party harness already in bashy's own fleet
   all own loops. None of them owns a pure-Go, Windows-native, conformance-tested,
   confinable shell with an audited execution pipeline. A second harness competes with
   everyone; a substrate is bought by everyone.
4. **A Go-coded harness inside bashy would compete with the fleet's own first-party
   harness.** The fleet's measured conclusion about that harness was explicit: it lost
   the bake-off and is kept for its **event channel**, not its reasoning. Building a
   *second* in-house loop repeats an experiment whose result is already recorded. A
   *kit* does not repeat it — an authorable loop competes with nobody, and the
   first-party harness becomes one composition of the kit rather than a rival to it.
5. **The gap list does not ask for a loop.** Re-read §4: not one **GAP** row is blocked
   on owning inference. Every one is a socket, a gate, or a record.

### 5.2 Why "every command is a plugin, bash is the plugin system" is nonetheless right

It is right, and today it is **false in the code**. `dsh`'s claim is backed by four
properties bashy lacks:

| Property | `dsh` | bashy today |
|---|---|---|
| A registry with a declared contract | `ctx.tools.register()` | one `switch` with 76 clauses |
| Reversible registration | `ctx.effect()` returns a disposer | compile-time `append` |
| Composition from data | `cordis.yml` + profiles + patches | recompile |
| Third-party installability | `dsh plugin add`, discovery topic | none |

bashy has the *seams* — an atlas that already types every command by group, tier,
capability and effect; a middleware chain; a fleet registry with a `kind` discriminator
already carrying `cli | func | web | system`. **It has the seams and not the sockets.**

That is the work. It is bounded, it is architecturally native to what bashy already is,
and it delivers the user's stated vision literally: bash's own dispatch becomes the
plugin system, and a verb becomes an installable thing.

### 5.3 The position to claim

> **BASHY — Bashy's Agentic Shell Harness Yoke.** The governed execution floor every
> harness runs on, the conductor that runs harnesses, and the kit you build one with.

Four roles, in priority order:

1. **Substrate (own it).** Every shell-resolved external command a harness runs passes
   bashy's pipeline — recorded, advised, learned-from, and (new) *gated* and *confined*.
   P0 extends that same governance contract to front-door verbs. This is the role nobody
   else is playing.
2. **Conductor (already leading).** The fleet plane. Keep extending; it is ahead.
3. **Kit (the new one — §5.4/§5.5).** Ship the primitives that make a harness
   *authorable*: one model-call verb, an atlas-informed prompt catalog plus a stable
   `bash{command}` tool schema, and a language with
   structs, errors and channels. bashy does not ship an opinion about the loop; it ships
   the parts and one worked example.
4. **Peer (become bidirectional).** Speak the protocols in both directions so an editor
   or another harness can drive bashy's fleet, and so bashy can be mounted as another
   harness's shell backend.

### 5.4 What Bash++ changes — the part the first version missed

The first version of this analysis treated bashy's authoring language as out of scope.
That was the error. Here is the argument it skipped.

**Every serious agent harness is written in TypeScript, Python, or Go — and none in
bash.** That is not an accident of taste. A harness loop is built out of six constructs,
and stock bash can express none of them well:

| What a loop needs | plain bash | Bash++ |
|---|---|---|
| Structured messages, tool schemas, results | strings + `jq` round-trips | `type … struct`, maps, selectors |
| Parallel subagents with results collected | `cmd &` + `wait`, results via temp files | `go worker(arg)`, channels |
| Errors from a model call, propagated | `$?`, `set -e`, `\|\|` | `v, err := f()` / `if err != nil` |
| Cleanup on abort mid-turn | `trap … EXIT` | `defer cleanup(x)` |
| Streaming with backpressure | FIFOs, coprocs | `make(chan T, 16)`, `<-`, `select` |
| Composition across files | `source` | Go-form `import` |

Those are exactly the rows in `docs/bashpp-dag-tiered-orchestration.md` §2. Read them
against the list of things a harness does and the conclusion is hard to avoid:
**bash cannot express an agent loop; Bash++ can.** That single sentence is the whole of
the user's claim, and it is correct.

It also explains something the first version noticed but could not account for: why
bashy's ExecHandler chain is *structurally isomorphic* to `dsh`'s tool pipeline. It is not
a coincidence of design taste. A harness pipeline and a shell dispatch ladder are the same
object; bashy arrived at it from below. Bash++ is what lets the rest of the harness be
written at the same altitude.

**Honest implementation status — the argument is sound and roughly 1% built:**

- `LangBashPP` is *reserved* in the parser (`sh/syntax/parser.go:87–100`) as a variant
  that is deliberately **byte-identical to bash**: `bashpp_test.go` is a superset gate
  asserting the same AST, the same errors, and the same printed output across the shared
  corpus. That is the right first move and it is the only move made.
- **No extended grammar exists.** No `:=`, no `go`, no `chan`, no `defer`, no `struct`.
- `--bashpp` is not wired into the CLI: `bashy --bashpp -c 'echo hi'` returns
  `flag provided but not defined: -bashpp`.

So the language is a **design of record with a reserved slot**. Nothing in this section is
a claim about what bashy can do today.

### 5.5 The one missing primitive, and the one condition on the name

Inventory what a harness kit needs against what bashy already ships:

| Harness ingredient | bashy today |
|---|---|
| Tool catalog with typed effects | **have** — 292 atlas entries, `{read,write,destroy,net,exec,cred,priv,remote,persist,spend}` |
| Guarded dispatch pipeline | **partial** — ExecHandler chain for shell-resolved externals; P0 adds deny/ask and front-door coverage |
| Confinement | P2 |
| Subagents, delegation, gating, boards | **have** — the fleet plane, 15+ verbs |
| Skills / procedural knowledge | **have** — `skills`, `craft` |
| Retrieval / memory | **have** — `kb`, `recall`, `graph` |
| Graph / DAG execution across tiers | **have** — `bashy dag`, plus the tiered emitter design |
| Durable record | partial — `execlog` (argv/exit); P5 |
| Loop-authoring language | **Bash++** — designed, unbuilt |
| **Model call with tool-calling** | **absent — nothing in the tree does this** |

The last row is the gap, and it is one row. Not a loop, not a session store, not context
management, not a TUI: **one request/response**. JSON messages and tool schemas in; an
assistant message and tool calls out. Effects `{net,spend}` — which places it exactly
where `judge`, `meet` and `delegate` already sit, *outside* the enforced local-first
lifecycle list, and exactly where `philosophy.md` §3 already concedes inference lives:
*"The one thing bashy cannot compute for itself: inference."* Adding a verb that CALLS a
model does not violate that; it is the honest expression of it.

Build that verb and prove the plain-bash worked loop, then land its P6 Bash++ port, and
the sentence "bashy is an agent harness kit" becomes true — not because bashy grew a
privileged loop, but because writing one stopped requiring another harness.

**The condition on the name.** BASHY-as-recursive-acronym is a good name for this
architecture and a premature name for today's binary. The umbrella's public-release
claim ledger tracks exactly this failure mode, and "agentic shell harness" read by
a stranger promises a thing that runs an agent, which bashy currently does not — it runs
*harnesses*. Use the name internally now, in design docs and in the plan; publish it when
P0.5 has shipped and P6's Bash++ port of the worked loop has closed the same gate. The acronym then describes
something a reader can run, which is the only kind of claim the ledger allows.

---

## 6. The plan

Phases are ordered by (value ÷ effort), and each names its gate. Nothing here requires
an agent loop; nothing here reaches for the network in a lifecycle verb.

### P0 — The policy plane: something that can say no
*Closes gap rows 7 and 9. Highest leverage in the document.*

Today bashy's pipeline can record, advise and warn. It cannot **deny**. `docs/audit-log.md`
already says so and already names the missing piece: *"This records; it does not block
(that is the policy engine's job)"*, and one section later, *"meaningful when the policy
engine ships."* A substrate that cannot refuse is a logger. **P0 is the component bashy's
own docs are written against.**

- Add a shared governance entry point used both by the ExecHandler chain and the
  front-door dispatcher; the policy middleware sits inside `audit` and outside `advisor`
  (`agentos.go:1562–1618` is the existing shell composition site), with three
  verdicts — `allow` / `deny` / `ask` — where `ask` routes through the existing
  `bashy ask` channel (it already solves the hard part: reaching a human whose stdin the
  harness owns).
- Rules key on what the atlas already knows: **effects** (`write`, `net`, `exec`, `cred`,
  `spend`, `priv`), group, tier, and argv patterns. No new vocabulary invented.
- Ship a `deny` for the one class every harness gets wrong: a write to a path outside
  the session workspace when a workspace is declared, and a stale-write (edit of a file
  whose mtime moved since the agent read it — row 9, closed for free at the same seam).
- Off by default, `BASHY_POLICY` gated, never in `cmd/bash`, never in `--posix`.

**Gate:** a scripted agent session in which a denied command returns a structured refusal
the model can act on, the audit chain records the denial, and the same session with the
policy off runs it. Plus: the existing 86/86 and `go test ./...` unchanged.

**Non-goal:** a general-purpose policy language. Three verdicts, atlas-keyed rules, one
file.

### P0.5 — `bashy llm`: the harness-kit primitive
**Design of record: [`plan-bashy-llm.md`](plan-bashy-llm.md).**
*Closes §5.5's one gap. The phase the Bash++ argument adds, and the smallest change in
this document with a category-level payoff.*

One verb: messages + tool schemas in, assistant message + tool calls out. Everything a
harness needs beyond this, bashy already has.

- **Scope is one request/response.** No loop, no session store, no context window
  management, no compaction, no streaming UI. Those belong to whoever authors the loop —
  which is the point.
- **Shape:** `bashy llm --model NICK` reading a `bashy-llm-v1` request on stdin
  (messages, optional tool schemas, optional response format) and writing the response
  envelope on stdout. The atlas types *effects*, not *arguments*, so it supplies a
  rendered **prompt catalog** plus **policy keying**, not generated per-verb
  JSON-Schema. The model-facing tool stays the one every model is trained on:
  `bash{command}`. See `plan-bashy-llm.md` §5.
- **Provider seam, not a provider.** Model resolution reuses `bashy models` / `bashy
  agents` (bands, nicknames, `tool:model` bindings, ring precedence) and credentials come
  from `bashy secrets`. A local Ollama model and a hosted endpoint are the same call.
  This is the equivalent of `dsh`'s `ctx.llm` — one seam, several providers — expressed
  as a command rather than a service.
- **Effects `{net,spend}`**, sitting beside `judge`/`meet`/`delegate`, and explicitly
  **not** added to the local-first lifecycle list. The effects conservatively describe
  what the verb may do; with the T0 Ollama provider it reaches only the local runtime and
  incurs no metered spend, so an inference-driven loop still runs in the air-gapped room.
- **Governed through P0's shared entry point:** direct front-door dispatch bypasses the
  current ExecHandler chain, so P0.5 must explicitly enter the shared governance path.
  Only then is the call audited and refusable by effect (`spend` is an effect a rule can
  deny); native GenAI spans remain the call implementation's responsibility.

**Gate:** an agent loop written first in plain bash + `jq` — read task, call `bashy llm`
with the atlas prompt catalog and the `bash{command}` schema, dispatch the returned tool
calls through bashy, feed results back, terminate on a `bashy gate` verdict — runs a real
repository task to a green gate, with the entire loop visible as a script the user can
edit. It must also run against local Ollama without external network access. P6 ports the
same worked task to Bash++; P0.5 cannot depend on a language phase sequenced after it.

**Non-goals:** no session/context service, no streaming TUI, no compaction, no provider
of our own. If the loop needs to remember something, it writes a file — `kb`, `execlog`
and the P5 record are already there.

### P1 — Hook-bridge consumption: govern harnesses bashy did not launch
*Closes row 17. Cheapest large win available.*

`install-agent` points a harness's shell at bashy — good, but it only governs what the
harness routes through a shell. Claude Code and Codex both ship **hook protocols**, and
`dsh` already consumes both (`hooks-claude-code`, `hooks-codex`, over a shared
`hook-protocol` library). A `PreToolUse` hook is a *deny point for every tool the harness
has*, including its native file editor — which bashy's shell seam never sees.

- Implement the two hook wire protocols as a bashy verb (`bashy hook <vendor>`), so
  `install-agent` can wire a harness's hook config to bashy's P0 policy plane.
- The same bridge feeds `execlog`/`audit` the tool calls that never touched a shell —
  which is a partial, cheap answer to row 4.

**Gate:** with the hook installed, a harness's *native* file write is recorded in the
audit chain and refusable by policy; `bashy install-agent --check` verifies the wiring.

### P2 — Confinement: `bashy sandbox` at the command, not the container
*Closes row 8.*

`dsh` demonstrates the shape: one seam, platform runners probed in order, **fail closed**
with an explicit unavailable error rather than silently running unconfined, and
enforcement completeness (`full` / `partial`) reported as a fact rather than assumed.

- Add a confinement seam consumed by the P0 policy verdict `confine`: Landlock (Linux),
  Seatbelt (macOS), restricted token (Windows), with bwrap preferred when present.
- Modes mirror what agents actually need: `read-only` and `workspace-write`.
- Licensing note: implement or invoke, never vendor a non-permissive launcher. A
  download-and-exec runner is allowed under existing policy; a linked one is not.
- Keeps the naming discipline: `sandbox` stays the container tier; this is a *policy
  effect*, not a new tier.

**Gate:** an escape attempt (write outside the workspace root) fails under each platform
runner, and an unavailable runner produces a loud refusal, never a silent pass.

### P3 — The verb socket: make "everything is a plugin" true
*Closes rows 6 and 20 — the user's actual proposition.*

Turn the 76-clause `switch` into a **registry with a manifest**, without giving up the
compile-time guarantees that make the atlas trustworthy.

- Define a verb manifest (`kind: verb` in the existing fleet asset vocabulary — it
  already discriminates `cli | func | web | system`): name, atlas entry (group, tier,
  capabilities, **effects**), and an implementation that is a binary, a `dag` target, or
  a skill.
- Resolution order mirrors the existing ring precedence — embedded → shared → cloud →
  local — so a built-in verb always wins over an installed one unless explicitly
  overridden, and the atlas coverage tests keep failing the build for a built-in with no
  entry.
- `bashy plugin {list,add,remove,show}`; a third-party verb appears in
  `bashy commands --atlas` with its declared effects, is subject to P0 policy like any
  other, and **cannot** declare `net` if it claims a lifecycle-loop group (the existing
  local-first test extends to installed verbs — this is the property that makes the
  socket safe to open).
- Middleware becomes registrable the same way, with a documented ordering contract, so
  an installed plugin can observe without patching `agentos.go`.

**Gate:** an out-of-tree verb installs, dispatches, appears in the atlas with correct
effects, is denied by a policy rule, and is refused at install time when it declares
`net` in a loop group.

### P4 — Be drivable: the server halves
*Closes row 19.*

bashy is ACP-client and A2A-client. Add the mirrors:

- **ACP server** — `bashy` presents itself as an agent to an editor. Its "model" is the
  fleet: a prompt becomes a `delegate`/`weave` run, gate-settled before it reports
  completion (the `herald` design already establishes the rule that a self-reported
  completion is a claim, not evidence — reuse it verbatim).
- **A shell-backend mode** — a documented way for another harness to mount bashy as its
  execution backend. `dsh`'s `ctx.shell` seam is the concrete target and its provider
  README enumerates the limitations bashy fixes (unconfined, no persistence, POSIX-only,
  Windows unsupported). A `dsh-bashy` provider is a small, high-visibility, MIT-compatible
  demonstration that the substrate thesis is real.

**Gate:** an editor speaking ACP drives a fleet run end to end; a `dsh` composition runs
its bash tool through bashy on Windows, where its own provider cannot run at all.

### P5 — The record: verbatim sessions
*Closes rows 4 and 5. Sequenced last here only because it is already owned elsewhere.*

`execlog` records argv and exit but **never stdout**, so no finding an agent made is ever
captured, and the only artifacts holding agent reasoning are harness transcripts on an
expiry timer. `dsh` solves this with one enforced invariant — *model-visible ⟺ logged* —
and it is the invariant bashy is missing.

The design work here is already recorded in the cross-cutting notes (a verbatim session
mirror, byte-capped, replacing a proliferation of stores that stopped writing silently).
**This plan defers to that design rather than proposing a competing one**, and only adds
one requirement from the comparison: whatever is built must be **searchable** (`dsh`
ships `session_search` / `session_trace`; a store nobody can query is the same as no
store), and it must fail loud when it stops writing — the failure mode already observed.

### P6 — Bash++ grammar, staged behind the superset gate
*Makes P0.5's loop worth writing in Bash++ rather than in bash with `jq`.*

The parser variant already exists and is gated byte-identical to bash. Stage the grammar
in the order a harness loop needs it, each stage keeping that gate green:

1. **Values** — `:=`, `var`, `type … struct`, maps, selectors. Kills the `jq` round-trip
   that makes a message list unbearable in bash. Biggest single ergonomic win.
2. **Errors** — multiple returns and `v, err := f()`. A model call has two outcomes and
   `$?` conflates them.
3. **Concurrency** — `go`, `make(chan T, N)`, `<-`, `select`, `defer`. This is what turns
   "fan out to five agents and take the first that passes a gate" from a temp-file
   protocol into four lines.
4. **Modules** — `import`. Makes a loop composable into a library of loops.

Stages 1–2 are enough to author a credible harness; 3 is what makes it *better* than one
written in TypeScript, because the subagents being fanned out are processes, and processes
are what a shell was always for.

**Gate, at every stage:** `TestBashPPMatchesBash` stays green (a bash program parses,
prints and errors identically under `LangBashPP`), `make test-bash` stays 86/86, and the
POSIX certification arm is untouched — `--posix` alone still selects the certification
baseline. **The compatibility contract is the product; the grammar is the extension.**

### Sequencing and parallelism

```
P0   policy ──┬── P1 hooks      (P1 consumes P0's verdicts)
              └── P2 confine    (P2 is a P0 verdict)
P0.5 llm    ──┬── P6 bash++     (P6 is what makes P0.5's loop pleasant to write)
              └── P3 socket     (a loop is the first thing anyone will want to install)
P4   server ──── independent    (the ACP server can front a Bash++ loop instead of a run)
P5   record ──── independent; owned by the existing context/memory design
```

**Two keystones, and they are independent of each other.** P0 governs what a loop is
allowed to do; P0.5 makes a loop writable at all. Run them in parallel — different files,
different risk, no shared surface. P0.5 + P6 is the "harness kit" thesis; P0 + P2 is the
"governed substrate" thesis; **the product is both, and neither one waits on the other.**

If only one phase is ever built, build **P0.5** — it is the difference between a document
that argues bashy could be a harness and a repository that demonstrates it.

### Explicit non-goals

- **No agent loop in Go.** A loop is a Bash++ program the user can read and edit, never a
  compiled opinion. Row 1 stays **N/A** for bashy's own code and moves to *authorable*.
- No context compaction, no prompt-assembly service, no session/context store beyond P5.
  Rows 3 and 16 stay as they are — a loop that needs them writes files.
- P0.5 is a **provider seam, not a provider**: bashy ships no model and hosts no
  inference. Row 2 becomes a *call*, not an owned capability.
- No chat TUI competing with the harnesses in the fleet. `bashy chat` stays a *governed
  launcher* for a third-party CLI's native UX.
- No MCP client (row 18) until something concrete needs it. bashy's answer to "how does
  a model reach a capability" is the shell, and 292 atlas entries is a larger tool
  catalog than any MCP server ships. Revisit only if a target harness can consume MCP
  but cannot consume a shell — which no current harness satisfies.
- No vendoring of a non-permissive confinement launcher (P2), and no plugin mechanism
  that lets an installed verb escape the atlas effect declaration (P3).

---

## 7. What would change this recommendation

Stated plainly, so it can be checked rather than argued:

1. **A local model good enough to conduct.** If a locally-runnable model can reliably
   act as an L3 conductor, an inference-driven loop stops being a `net` dependency, the
   local-first objection weakens further, and P0.5's `{net,spend}` effects become
   `{spend}`-free for the local provider. Watch the band ladder. *(Partially in play
   already — this is one of the reasons P0.5 is cheap: `pkg/ollm` is a local provider
   waiting for a caller.)*
2. **Harnesses closing their shell seam.** If the major harnesses stop routing commands
   through a configurable shell, the substrate position loses its reach and the hook
   bridges (P1) become the only channel — which would raise P1 to P0.
3. **A substrate incumbent appearing.** If a harness vendor ships a governed, conformant,
   cross-platform execution floor as a separate product, the differentiation collapses to
   bash fidelity alone and the calculus changes.
4. **Adoption failing at the shell.** If, after P0–P2, no harness other than the
   first-party one is measurably running on bashy, the substrate thesis is falsified by
   measurement and the conductor role becomes the whole product.
5. **The worked-loop gates failing.** If the P0.5 plain-bash loop cannot carry a real
   repository task to a green gate, dispatching model-proposed commands through the
   substrate loses too much fidelity and the kit thesis is falsified. If that loop works
   but its P6 Bash++ port does not, the kit survives but the Bash++ authoring thesis does
   not. Run both gates before adopting the acronym in public.

---

## 8. Evidence appendix — what was verified, and how

Verified directly in this session (commands run against both trees):

- `dsh` package inventory, service list, tool catalog: `docs/capability-seams.md`,
  `docs/tool-catalog.md`, `docs/architecture.md`, `docs/tool-execution-pipeline.md`,
  and 30+ package READMEs. Counts computed by `grep -c` over the generated catalogs.
- bashy has **no** chat-completions client and **no** tool-call handling anywhere in its
  tree or coreutils: searched for `chat/completions`, `tool_calls`, `"tools":`,
  `ToolCall`, and every mainstream provider SDK. Only hit is inside the vendored Ollama
  fork's own Anthropic-compat server.
- bashy's turn observation declares itself incomplete:
  `subprocessHarnessCoverage = "subprocess_harness_turn"`, `CoverageComplete: false`.
- ACP is client-side (launches a subprocess and connects as the *client* side of the
  connection); `herald` is an A2A *client* dialing a peer URL.
- No Landlock / Seatbelt / bwrap primitive outside vendored podman seccomp defaults.
- The middleware chain and its ordering rationale, read in full at its composition site.
- 292 atlas commands; 76 top-level dispatch clauses; no `plugin` verb.

Verified in the revision pass (Bash++ claims):

- `LangBashPP` exists in `sh/syntax/parser.go:87–100`, in `langBashLike` and
  `langBashExact`, with `TestBashPPMatchesBash` gating byte-identical parse/print/error
  against `LangBash` across the shared corpus.
- **No extended grammar is implemented** — no `:=`, `go`, `chan`, `defer`, or `struct`
  handling anywhere in `sh/syntax` or `sh/interp`.
- `--bashpp` is not wired into the CLI: `bashy --bashpp -c 'echo hi'` →
  `flag provided but not defined: -bashpp`.
- Bash++ design of record read in full: `bashpp-posix-superset-syntax.md` (553 lines) and
  `bashpp-dag-tiered-orchestration.md` (203 lines).

Not verified: `dsh` was **not built or executed** — no `pnpm install`, no run. Every
`dsh` claim above comes from its source and generated documentation, which is unusually
precise but is still documentation. Before committing to P4's `dsh-bashy` provider,
build `dsh` and run its bash tool once; a provider written against a README is a
provider written against a claim.
