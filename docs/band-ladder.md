# The band ladder — L1–L4 across every provider

*Built and live-verified 2026-07-13. Registry: `coreutils/pkg/fleet/baseline/`.*

## Bands map to roles

| band | role | the work |
|---|---|---|
| **L1** | **QA / verify** | mechanical, objectively checkable — run the tests, report the result |
| **L2** | **coding** | scoped implementation against a spec |
| **L3** | **conductor** | orchestration, disciplined looping, tool use |
| **L4** | **steward** | judgment, ambiguity, human partnership |

**A band is a FLOOR, not an identity.** An L4 model satisfies an L3 request — and
that is exactly why the cap matters: it *can*, but doing so burns money and tokens
for nothing. So a role declares a **minimum band**, and the router takes the
**cheapest agent at or above it**. Sending a steward to clear an intern's ticket is
the over-spend the whole abstraction exists to make visible.

## The ladder

Every rung below was **live-probed** (`bashy agents verify --live`) — it launches
and answers. 19 of 20 agents pass; `opencode:kimi-k2.7-code` fails on opencode's own
opaque `UnknownError` (it fails standalone too).

| band | Anthropic (claude) | OpenAI (codex) | Google (agy) | DeepSeek (aider) | Moonshot (aider) |
|---|---|---|---|---|---|
| **L4** steward | `fable5` · `opus4.8` | `gpt5.6-sol` · `gpt-5.5` | — | — | — |
| **L3** conductor | ↑ | `gpt5.6-terra` | `gemini3.1` ⚠ | `deepseek-v4-pro` ⚠ | — |
| **L2** coding | `sonnet5` | `gpt5.6-luna` | `gemini3.5-flash` | `deepseek-v4-flash` | `kimi-k2.6` · `kimi-k2.7-code` |
| **L1** QA | `haiku4.5` | `gpt5.3-spark` · `gpt5.4-mini` | `gemini3.5-flash-low` | `deepseek-chat` | — |

⚠ **`gemini3.1` and `deepseek-v4-pro` are the two open questions.** They are their
vendors' *reasoning* tiers, pegged L3 as a **declared guess**, and **nobody has run
either as a conductor**. They are the only rungs where the L2-vs-L3 answer changes
who is allowed to conduct — so they are the first two candidates for the ladder
bake-off.

Confirmed by the operator's lived experience: **`opus4.8` and `gpt-5.5` serve both
conductor AND steward** (hence L4), and **Kimi is a coder** (L2). Anthropic and
OpenAI are the only providers with a *confirmed* L4.

## The calibration rule

> **A provider's own tier ladder is NEVER mapped positionally.**

A vendor's flagship is a claim about *that vendor's lineup*, not about this fleet's
ladder. `Gemini 3.1 Pro` and `DeepSeek V4 Pro` are their vendors' top reasoning
tiers — and whether that makes them L3 **here** is exactly the thing nobody has
checked.

**A retraction worth recording.** I briefly demoted both to L2 and stamped it
`band_source: operator`, on the strength of an operator remark that turned out not
to cover the *Pro* tiers at all. That is the same failure as everything else in this
registry's history: **a number that nothing checked, written down as though
something had.** Retracted the same hour. They are back at L3 `declared`, flagged
untested, and first in the bake-off queue.

The Anthropic anchor is what bands are measured against, and the anchor bias is
disclosed rather than hidden.

## `~` means the band is not yet measured

```
opus4.8    L4~     <- the tilde is the point
```

| `band_source` | what it means |
|---|---|
| `declared` | a considered guess from vendor tier + priors. **Nothing has tested it.** |
| `operator` | pegged from an operator's lived experience across real runs. Not a controlled experiment, but evidence from work that actually shipped. |
| `measured` | earned by running the model up a difficulty ladder to the rung where it **failed**. |

**Every band in this fleet is currently `~`.** None has been measured.

That honesty is the whole point. This registry has already spent months trusting
numbers nothing had ever checked — bands scored against bindings that were dead,
and an `operability: 0.996` on an agent that could not execute a single turn. One
tilde is a cheap way to never do that again.

## Why a quiz cannot measure a band

It was tried. Five reasoning tasks, all seven operable agents: **everyone scored
5/5.** An L1-difficulty question cannot distinguish an L1 model from an L4 one.

> **Success at a low band is not evidence of a high band.**
> **A band is the highest rung a model reliably CLEARS — not a score.**

So the only valid instrument is a **ladder bake-off**: run each model up increasing
difficulty until it fails, and peg it where it stops. Real task, isolated workspace,
objective gate — did the run converge, pass the gate, survive the judge, leave a
clean tree. Not a Q&A set, and not another model's opinion.

That needs **write authority**, which is why it has not been run. Until it is, every
band here is a `~`.

## Adding a rung

```
bashy models add <name> --family F --version V --band N \
    --provider P --kind subscription|api --upstream <the id the TOOL accepts>
bashy agents add <tool>-<model> --tool T --model M
bashy agents verify --live <agent>      # NON-NEGOTIABLE
```

The upstream id is **whatever the tool actually accepts**, and it is often not the
slug you would guess:

- agy takes **display strings**: `Gemini 3.1 Pro (High)`, not `gemini-3.1`.
- DeepSeek rejects `deepseek-v4` — it is `deepseek-v4-pro` / `-flash`.
- codex takes `gpt-5.6-sol`, not `sol`.

Every one of those was a dead binding until something launched it. **Live-probe or
it did not happen.**
