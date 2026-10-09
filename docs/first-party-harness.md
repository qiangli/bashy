# The first-party harness — ycode in the fleet

*Registered and live-verified 2026-07-13.*

## The question

*Should we build out ycode to take over opencode and aider as the harness for
third-party API-key services?*

## The answer: it was never a build

ycode is **already a CLI agent harness.** Its own help says so, and the code backs
it:

| | |
|---|---|
| `internal/runtime/conversation/` | 1489-line agent loop — executor, budget/cost, **loop detection**, delegation, memory, inference routing |
| `internal/tools/` | **111 files** — the tool surface |
| `internal/bus/` | an event bus — *this is the steering channel* |
| `internal/eval/` | "evaluation framework for agentic capability regression testing" |
| `internal/api/models.go` | provider maps for anthropic · openai · gemini · xai · dashscope · **moonshot** · **deepseek**, each to its own env key |
| `ycode prompt --model M --print` | the headless one-shot entry the fleet needed |

**The hard part — the agent loop — exists.** What was missing was a *launch
contract*. It's now in the registry, with five API-key bindings, **all live-probed**:

```
ycode:deepseek-v4-pro    ycode:deepseek-v4-flash    ycode:deepseek-chat
ycode:kimi-k2.7-code     ycode:kimi-k2.6
```

**Fleet: 21 of 21 agents live** — every binding, every provider, every harness.
*(21, not 26: aider was retired from the lane on 2026-07-14 — see below.)*

*(An earlier version of this line said "it reaches kimi, which opencode cannot."
That was also wrong: opencode reaches kimi fine. The registry had the wrong provider
id — opencode calls it `moonshotai`, not `moonshot` — and opencode died on an opaque
UnknownError that named nothing. I blamed the tool; the typo was ours.)*

## Why this is the right line

| model kind | harness | why |
|---|---|---|
| **subscription** (claude, codex, agy) | **the vendor's CLI** | the CLI *is* the product — flat-rate billing, day-one model support, a loop the vendor tunes. Owning this is pure loss. |
| **API-key** (deepseek, moonshot, …) | **ycode** | there is no product here. It's an HTTP call. A third-party CLI in this path is a liability with no upside. |

The vendor CLIs also remain the **cross-check** against a first-party bug — which
is the real answer to the harness-monoculture risk. Keeping *aider* was never the
answer; keeping *diversity of implementation* is.

## RETRACTED: "the third-party harnesses cannot be steered"

**This document originally argued that aider and opencode were *architecturally
incapable* of steering, and that this was the decisive case for a first-party
harness. That was wrong, and it was wrong because I never tested it.**

All five are steerable. Measured, through the real control socket:

| harness | steer |
|---|---|
| claude | `STEERED_OK` |
| codex | `STEERED_OK ×3` |
| agy | `STEERED_OK ×54` |
| opencode | `STEERED_OK ×7` |
| aider | `STEERED_OK ×93` |

I had only ever tested the launch **in the registry** — `opencode run`,
`aider --message` — which are one-shots with nothing to interrupt. Their interactive
launches (`opencode`, `aider` with no `--message`) accept a steer perfectly well. I
wrote down a conclusion about a launch I had not attempted, which is the exact
failure this fleet keeps making.

## So what IS the case for ycode?

The honest one, with the false argument removed:

- **Evidence by construction.** Tool calls as structured events, not stdout scraped
  and guessed at. This is what the fleet-evidence invariant has been asking for.
- **Token streaming.** `stream=true` in a loop you own; impossible through a CLI.
- **No contract drift.** Five dead bindings in one day came from other people's CLIs
  changing their flags, model ids and provider names underneath us.
- **In-flight effect caps.** Refuse the fifth write, not just the launch.

That is still a strong case. It is **not** the case I made, and the difference
matters: a capability gap would have been decisive on its own. These are trade-offs.

## And ycode gets the headless contract right

```yaml
exec: ycode prompt --model {model} --print {prompt}
```

`--print` is an **explicit** flag. claude infers print mode from *"is stdout a
terminal"* — which is correct on a pipe and **false the moment you give it a PTY**,
at which point it opens its REPL and sits there forever. ycode does not guess, so a
terminal can be handed to it (to steer it) without changing what it does.

> **A terminal changes what an agent can be ASKED. It must never change what the
> agent DOES.**

## The registry bug this exposed

**The id a model answers to is a property of the TOOL, not of the model.**

```
aider/opencode   deepseek/deepseek-v4-pro    litellm wants provider/model
ycode            deepseek-v4-pro             it detects the provider itself
agy              Gemini 3.1 Pro (High)       a display string, not a slug
```

The registry stored **one global `UpstreamID`** — and whatever you put in it is
wrong for somebody. Every ycode binding was dead on arrival: the registry handed it
litellm's prefixed form, ycode rejected it, and *the same model worked perfectly
when ycode was run by hand.*

Fixed with a per-tool override, and the launcher now resolves the id **after** the
tool is known:

```yaml
model: deepseek/deepseek-v4-pro   # default
ids:
  ycode: deepseek-v4-pro          # this tool wants it bare
```

That was the **fourth** dead binding of the day, and `agents verify --live` caught
it within sixty seconds of ycode being registered. The **fifth** was the same shape:
opencode wants `moonshotai/kimi-k2.7-code`, and the registry said `moonshot/`.

One model, three spellings, all live:

```
aider (litellm)  moonshot/kimi-k2.7-code
opencode         moonshotai/kimi-k2.7-code
ycode            kimi-k2.7-code
```

## And bashy was destroying opencode's config

`ApplyTrustPreseed` did a blind `os.WriteFile` of a permissions-only blob over any
existing `opencode.json` — **taking the project's `provider` block with it.**
opencode reads its model *endpoints* from that file, so bashy silently deleted the
configuration the agent needed, and the agent then failed with a server error
pointing nowhere near the cause.

The claude preseed has always merged. This one did not, and nobody noticed **because
the failure surfaced as somebody else's bug.** It now merges, refuses to overwrite a
config it cannot parse, and never overrules a permission the project set itself.

## Still owed — nothing. All four shipped 2026-07-14.

This section used to list four things. Here is what happened to each.

### 1. Steering — DONE, and it never needed the bus

ycode steers. It always did. It is a bubbletea TUI reading stdin, so bashy's pty
control socket reaches it exactly as it reaches codex and opencode. Measured
through the real socket (`STEER_TOOL=ycode go test ./pkg/agentpty -run
TestSteerLive`): `STEERED_OK`, **echoed AND answered**.

The registry said `supports_say: false`, with a comment asserting the bus had to
be exposed first. Both were written without ever launching `ycode` interactively
and typing at it. **All six harnesses steer.**

### 2. Token streaming — superseded by something better

Not a delta stream: a **turn boundary**. See below.

### 3. Evidence by construction — DONE, and it is the differentiator

`--events <path>` emits NDJSON on **both** paths (one-shot and TUI):

```
{"type":"turn.start","data":{"prompt":"..."}}
{"type":"tool.call","data":{"name":"read_file","input":{...}}}
{"type":"turn.end","data":{"status":"ok","text":"the answer"}}
```

`turn.end.text` equals exactly what `--print` writes to stdout — a consumer
compares them, and if they disagree one of us is lying.

**This is the thing no third-party CLI can give us.** Everywhere else, bashy decides
a turn is over by watching for **25 seconds of silence** (`chat.Session.WaitIdle`) —
wrong for an agent that pauses to think, wrong for one that renders a spinner, and
a 25-second tax on every turn. ycode just *says* when it is done, and bashy believes
it, because that is a fact the agent reported rather than a silence bashy interpreted.

Live, through `chat.Session`, on a real steerable session:

```
turn ended after 8.3s          (before: 172s — it never ended, it timed out)
Turn() = "example.com/probe"   (before: a raw ANSI terminal scrape)
```

`tool.call` arrives as **structured data**, which is what the fleet-evidence rule has
been asking for from the beginning.

**Not yet reached:** server mode. When `ycode serve` is running the agent loop lives
in the server process, which never sees the client's `--events`. Closing that means a
sink on the server side — a different design, not a missing call. Recorded in
`ycode/internal/wireevents`.

### 4. The A/B — RAN. And ycode did not win.

`bashy/docs/harness-ab-deepseek.md`. One model (`deepseek-v4-pro`), one task, one gate,
three harnesses:

| harness | wall | gate | code |
|---|---|---|---|
| **opencode** | **25s** | PASS | +23 |
| aider | 68s | PASS | +38 |
| **ycode** | 110s | PASS | +53 |

**ycode was the slowest and wrote the most code.** Say that plainly, because the case
for a first-party harness was never "it wins a bake-off" — it is the event channel
above, and that is worth having on its own.

The A/B earned its keep anyway: it found **two harness bugs, both ours.**

- **ycode had no write authority.** It produced NOTHING and exited 0. The obvious
  reading — "deepseek can't do this" — would have been a false verdict about a MODEL.
  It said so itself: *"I have the implementation ready, but I don't have
  workspace-write permissions in this environment to write the file."* Every other
  harness gets an approval-gate kill-switch; ycode's `--danger-skip-permissions`
  existed and the registry never used it.

- **aider never read the spec** — *"test file (not provided)... we need to deduce."*
  It only sees files explicitly added to the chat. **Retired from the lane** (2026-07-14):
  not on quality (it passed), on architecture. `bashy invoke` hands an agent a TASK, not
  a file list, because a conductor does not know which files a task will touch. An agent
  that must be told its files up front cannot be delegated to — only assisted.

And the finding that outlives all of it:

> **All three harnesses exited 0 when they failed.** Three harnesses, two catastrophic
> failures, zero non-zero exits. A pipeline gating on `$?` would have merged both.
> **A harness's exit code is not evidence. Run the gate.**

## Where this leaves the lane

- **subscription models** (claude, codex, agy) → **the vendor's CLI**. The CLI *is* the
  product: flat-rate billing, day-one model support, a loop the vendor tunes. Owning it
  is pure loss.
- **API-key models** (deepseek, moonshot/kimi) → **ycode** for anything a conductor
  delegates, because of the event channel. **opencode stays** as the cross-check —
  harness monoculture is the risk a first-party harness *creates*, and a second
  implementation is the only real answer to it. **aider is retired.**

Harness scores in the registry are still priors, with one exception: aider's `tool-use`
is now measured (0.7 → 0.4). **Live-probed is still not the same as good.**

## Genie preview workflow

`genie` is bashy's own agent: `bashy genie "MSG"` runs one headless turn on the
ycode engine that bashy links. It is a **preview tier**, and the workflow below is
the one documented path that is expected to work on macOS, Linux and Windows.
`scripts/genie-preview-workflow.sh` runs exactly these steps and exits non-zero
when any of them breaks.

**What "preview tier" means.** The surface works end to end today on the path
below and is exercised on all three operating systems, but it is not a
compatibility promise: flags, defaults and the builtin recipe may change between
releases, only the one-turn headless path is covered (no TUI, no `genie web`, no
long-running sessions), and a local model is proven for tool use only at the small
size the script pulls. Nothing here weakens Classic: `bash` and the coreutils
userland do not depend on genie.

### 1. Install

Download the single `bashy` executable for your OS and put it on `PATH`. There is
nothing else to install: genie, the model door and the Ollama engine launcher are
all inside it (the engine binary itself is fetched on first use). On Windows run
scripts as `bashy.exe scripts/genie-preview-workflow.sh`; no other bash is needed.

### 2. Pick a model

Every genie turn goes through the host's model door (`bashy llm up`, port 24556,
override with `BASHY_LLM_PORT`), so the model choice is one `-m` flag.

- **Cloud.** Either a door model (`door-*`, which rides a CLI seat you are signed in
  to) or a registry API model plus a provider key, for example
  `bashy model add mine --set kind=api,base_url=URL,model=ID,api_key_ref=MY_API_KEY`
  with `MY_API_KEY` in the environment or in `bashy secret`. The credential is
  read from the environment or the vault at run time and is never printed.
- **Local.** A small Ollama model that bashy pulls and serves itself:
  `bashy llm up` then `bashy ollama pull qwen3:1.7b`. Allow about 1.5 GB of free
  disk and memory; bashy refuses to start below its resource floors.

### 3. Run the scripted one-turn task

```sh
scripts/genie-preview-workflow.sh                  # local leg, then cloud leg
scripts/genie-preview-workflow.sh --legs local     # only the local model
scripts/genie-preview-workflow.sh --isolate        # throwaway BASHY_HOME and door port
```

Each leg asks genie, in a fresh directory, to create `hello.txt` whose only
content is a fixed token, run `cat hello.txt`, and show the output. The script then
checks the **file on disk** and that the answer contains the token; an answer
alone proves nothing. Every external step runs under a timeout
(`GENIE_PREVIEW_PULL_TIMEOUT`, `GENIE_PREVIEW_TURN_TIMEOUT`,
`GENIE_PREVIEW_UP_TIMEOUT`).

| Exit | Meaning |
|---|---|
| 0 | every requested leg passed |
| 1 | a leg failed, including a timeout |
| 2 | usage or environment error (no bashy, bad flag) |
| 3 | local passed but the cloud leg was skipped: no door model or credentialed API model on this host. **Not green for the preview bar.** |

On exit the script stops the door only if it started it, removes only the models
it added (the pulled tag and the `-bashy-<hash>` context variants genie derives
from it), and deletes its scratch directories. `--isolate` is the safe choice on a
shared host: it never touches an existing door, genie bundle or session.

### Known gaps

- `bashy llm down` cannot signal the door on Windows (`os.Process.Signal` with
  `SIGTERM` is unsupported there). The script works around it by killing the pid
  the isolated door logged; the proper fix lives in yoke's broker.
- A `door-*` model needs a live sticky binding, which goes stale (HTTP 409) when
  the CLI seat upgrades; the script therefore prefers a credentialed API model.
- The ollama model store is host-global, not under `BASHY_HOME`, so `--isolate`
  isolates the door and sessions but not pulled models; the script removes what
  it added.
