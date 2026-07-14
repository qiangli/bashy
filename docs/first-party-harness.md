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

**It reaches kimi, which opencode cannot.** Fleet is now 24 of 25 live.

## Why this is the right line

| model kind | harness | why |
|---|---|---|
| **subscription** (claude, codex, agy) | **the vendor's CLI** | the CLI *is* the product — flat-rate billing, day-one model support, a loop the vendor tunes. Owning this is pure loss. |
| **API-key** (deepseek, moonshot, …) | **ycode** | there is no product here. It's an HTTP call. A third-party CLI in this path is a liability with no upside. |

The vendor CLIs also remain the **cross-check** against a first-party bug — which
is the real answer to the harness-monoculture risk. Keeping *aider* was never the
answer; keeping *diversity of implementation* is.

## What the third-party harnesses cannot do

The decisive argument is not that opencode and aider are buggy. It is that they are
**architecturally incapable** of what this fleet is built around.

| harness | steerable? |
|---|---|
| claude | **yes** |
| codex · agy · aider · opencode | **no** |

`bashy meet say` — a chair correcting a rambling participant mid-turn — reaches
**exactly one of five harnesses.** `weave say` likewise. Effect caps are enforced at
*launch* (argv), because with a black box you cannot refuse the fifth write *in
flight*. Attestation evidence is **scraped from stdout and guessed at.**

None of that is fixable in someone else's CLI. All of it is trivial in a loop you
own — which is why `supports_say: false` on ycode is a **wiring job**, not a wall:
the bus is already there.

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
it within sixty seconds of ycode being registered.

## Still owed

1. **Steering.** Expose ycode's bus as a control socket → `supports_say: true`, and
   `meet say` / `weave say` start working for API models. This is the whole point.
2. **Token streaming.** Trivial with your own loop (`stream=true`); impossible
   through a CLI. It closes the gap `meet observe` currently has.
3. **Evidence by construction.** Tool calls become structured events instead of
   scraped stdout — which is what the fleet-evidence invariant has been asking for
   all along.
4. **The A/B.** `ycode:deepseek-v4-pro` vs `aider:deepseek-v4-pro` vs
   `opencode:deepseek-v4-pro` — same model, three harnesses, one gate. Anything that
   differs **is the harness**. Needs write authority.

Until (4) runs, ycode's harness scores in the registry are priors like everyone
else's. **Live-probed is not the same as good.**
