# ycode performance — L4 review

*2026-07-14. Participants: Sable (claude:fable5), Beatrix (claude:opus4.8),
Arlo (codex:gpt-5.5), Omar (codex:gpt5.6-sol). Secretary: Claude Code.*

**Outcome: 37% of ycode's wall clock was being spent re-deriving an answer it
already had. Fixed. 85s → 55s, and the flaky gate failure went with it.**

---

## The premise the meeting was convened on — and what survived it

The call was: *"ycode is ~5× slower than opencode. It is Go, opencode is Node.
This can only be an architectural flaw."*

Two parts of that did not survive first contact with a measurement, and the
meeting was told so in the brief rather than being left to discover it:

**The 5× was N=1 noise.** The original A/B ran ycode with **no write permission**
— it was doing something different. Three clean runs each put the real gap at
**2.8×** (ycode ~85s, opencode ~30s). Real, consistent, worth a meeting. Not 5×.

**Go-vs-Node cannot explain it, and the brief said so out loud.** In an agentic
loop wall-time is dominated by LLM round-trips; the harness's own compute is
microseconds. On a trivial prompt the two are *identical* — ycode 2.6s, opencode
2.6s. "Go should be faster" is true and irrelevant. The brief ruled it out
explicitly: *"if you find yourself proposing a goroutine, you have the wrong
hypothesis."*

This matters more than the fix. **A meeting convened on an unmeasured premise
spends four L4 agents arguing about a phantom.** Twenty minutes of measurement
before the invitation went out is what made the twenty minutes of meeting worth
anything.

## What the meeting found

All four read both codebases. The convergence was fast and the file:line evidence
was exact.

**Every continuation turn re-runs tool preactivation on an unchanged user message.**

The agentic loop re-enters `Turn()` once per LLM round-trip: ask → run tools →
append the *results* → ask again. `lastUserText()` walks back **past** the tool
results to the same original string every time. So preactivation gets identical
input on every turn.

And it is structural, not incidental — this is the part the meeting earned its fee
on:

- the cheap keyword/scoring tiers **skip tools that are already active**
- so on a continuation turn they find nothing new
- so `total` lands on **0**
- and `total == 0` is precisely the condition that **fires the expensive tiers**:
  a semantic vector query (2s timeout, `preactivate.go:201`) and, failing that, an
  **LLM classification call** (3s timeout, `preactivate.go:291` → a real provider
  request at `routing/classify.go:55`)

**Every turn. For an answer we already had.** The cheap path succeeding on turn 1
is what *guarantees* the expensive path runs on turns 2–25.

## Omar was right, and it cost me an assumption

I brought a number to the table: *"10 tool calls × ~3s = 30s of the gap."*

> **Omar (codex:gpt5.6-sol):** *"The 30s claim is not yet evidence-backed:
> `preactivate.go:201-204` establishes only a 0–2s **ceiling**... neither records
> actual duration. Also, **10 tool calls ≠ 10 turns** — one response may contain
> multiple calls executed together. Count iterations before multiplying."*

Both corrections landed. A timeout is a ceiling, not a measurement. And the real
turn count was **25**, not 10. My arithmetic was wrong in both factors and I would
have shipped a fix sized against a fiction.

**That is what an L4 seat is for.** The three agents who agreed with me were less
useful than the one who did not.

## The measurement (`YCODE_PERF=1`, one instrumented run)

```
turns (LLM round-trips): 25
preactivation TOTAL:     41.4s
wall:                    111.7s
as % of wall:            37%

msg_len=461 on every single turn — the input never changed
~1.9s per turn, 24 of 25 turns
```

## The fix

A memo on the input. Same message, same answer.

```go
if r.preActivatedFor == userMessage {
    return 0
}
r.preActivatedFor = userMessage
```

## Result (3 runs each, same task, same model, same gate)

| | before | after |
|---|---|---|
| wall (mean) | **85s** — 74.0 / 85.1 / 97.0 | **55s** — 75.2 / 41.8 / 48.4 |
| **gate** | **1 of 3 FAILED** | **3 of 3 PASS** |
| preactivation | 41.4s | **0.0s** |
| gap to opencode | 2.8× | **1.8×** |

**35% faster, and *more* reliable** — the flaky gate failure went with the waste.

## The honest caveat, stated rather than buried

Omar flagged the memo as **behaviour-changing**, and he was right.

On turn 1 the keyword tier hits, `total > 0`, and the classifier is **skipped**. On
turn 2+ everything it matched is already active, the cheap tiers return nothing, and
the classifier fires — so today it *can* activate tools on a later turn that it never
would have on the first. **The memo removes those late activations.**

The classifier was second-guessing a message whose keywords had already answered it,
24 turns running, at ~2s a turn. The **gate** is the only thing that can say whether
it was buying anything, and across three runs it says no: **3/3 pass, where the old
path passed 2/3.**

## What is left, and what it is NOT

**Remaining gap: 1.8×. It is TURN COUNT.** ycode took 8–20 turns across the post-fix
runs; turn count drives wall time. That is the next question and it is a *different*
one — it is about how the agent decides it is finished, not about per-turn overhead.

Deliberately **not** pursued on this evidence:

- **Tool-schema size.** ycode has ~111 tool files vs opencode's ~15, and the schema
  ships in every request. Plausible, and *unmeasured*. Before touching it: log
  `len(toolSpecs)` and the serialized schema bytes per request, both harnesses. Do
  not act on the file count — a file count is not a byte count.
- **System-prompt rebuild per turn.** Beatrix raised it; **Omar killed it** by reading
  opencode's source: it recomputes environment/instructions/MCP/skills inside its loop
  too. Not a differentiator.
- **Streaming.** Both stream. Not a differentiator.

## The strategic answer (agenda item 3)

The brief asked what else should move out of ycode into bashy. The meeting's finding
answers it in a way nobody proposed:

**The tool-routing cascade is not ycode's job — and it may not be anyone's.** It
exists to manage a 111-tool surface. opencode ships ~15 tools, sends them all, every
time, and has no router at all. It is faster and it converged on this task in fewer
turns.

The router is a **solution to having too many tools.** Before moving it to bashy,
ask whether the problem it solves should exist. A capability that is only needed
because of a self-inflicted surface is not a capability worth sharing.

## Actions

| # | action | status |
|---|---|---|
| 1 | Memo preactivation on the user message | **DONE** — `ycode 27ea6d8` |
| 2 | `YCODE_PERF=1` per-turn timing on stderr | **DONE** — it cost one run to find 41 seconds. Keep it. |
| 3 | Measure turn count: why does ycode take 8–20 turns where opencode takes fewer? | open — the whole remaining gap |
| 4 | Measure the *serialized schema bytes* per request, both harnesses | open — **measure before touching** |
| 5 | Decide whether the tool-routing cascade should exist at all | open — strategic, blocked on #4 |
| — | ~~System-prompt rebuild~~ | **killed in meeting** — opencode does it too |
| — | ~~Streaming~~ | **killed in meeting** — both stream |
| — | ~~Go-vs-Node~~ | **killed in the brief** — identical on a trivial prompt |
