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

**Fleet: 26 of 26 agents live** — every binding, every provider, every harness.

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

## Still owed

1. **Steering.** Expose ycode's bus as a control socket → `supports_say: true`.
   Note this is now *parity work*, not a differentiator: all five third-party
   harnesses already steer. ycode is the only one that does not.
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
