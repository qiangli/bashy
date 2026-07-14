# ycode performance — L4 review

*2026-07-14. Participants: Sable (claude:fable5), Beatrix (claude:opus4.8),
Arlo (codex:gpt-5.5), Omar (codex:gpt5.6-sol). Secretary: Claude Code.*

**Outcome: 85s → 42.6s (a 50% cut), 22 tool calls → 5, and the flaky gate failure
is gone. THREE bugs, and the meeting only found the first one. The other two were
found by putting a proxy on the wire and reading what the model actually received.**

| | wall | gate | tool calls |
|---|---|---|---|
| original | **85s** | 1 of 3 **FAIL** | **22** (16 bash: `base64`, `xxd`, hexdump) |
| + preactivation memo | 55s | 3/3 | — |
| + routing gate | 42.6s | 3/3 | 10–12 |
| **+ distill gate** | **42.6s** | **3/3** | **5** — identical every run |

Five calls is the minimal correct sequence: read the spec, read the stub, write the
function, run the tests.

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

## THE BIG ONE — which the meeting did NOT find

After the meeting's fix landed, I put a **counting proxy in front of the API** and
read what the model was actually being sent. This was sitting in the tool result:

```
[... 755 characters omitted ...]
```

`RouteContent` classifies a `read_file` result over 2000 chars as `RoutePartial` —
**keep the head, keep the tail, DELETE THE MIDDLE** (`session/routing.go:84-93`,
`partialContent` at `:165`). And it was called **unconditionally** from
`distillResults`: on turn 1, of an empty conversation, against a 64K-token window
with about 600 tokens used.

So the model asked for the test file it had been told to implement against, and we
handed it back **with the test cases — the entire specification — cut out of the
middle.** Its next seventeen turns:

```
cat → sed ranges → python → awk → base64 → base64|xxd → xxd hexdump → awk|tee
```

**It was not flailing. It was doing exactly what anyone does when handed a document
with the middle torn out.** Then we charged it seventeen round-trips to work around
us.

**Context management is a response to a context PROBLEM.** Below the soft threshold
there is no problem, and saving 800 characters costs the agent its ability to read.

## And the second opinion found the half I missed

codex:gpt-5.5, asked to *refute* the fix:

> *"The second instance is not another RouteContent; it is `DistillToolOutput` still
> running below pressure. It has no pressure check, no default read_file exemption...
> If the principle is 'don't damage tool observations without pressure', this path
> violates it too."*

He was right. My gate skipped `RouteContent` and then **still** called
`DistillToolOutput`, which head/tails at **1000 chars** for a non-caching provider.
The measurement had improved enough to hide it — 85s → 42.6s, no more `xxd`, gate
3/3. **It looked fixed.**

> **A measurement that improves is not the same as a cause that is gone.**

## What actually found these

Not code review — I had read `routing.go` and it looked reasonable. Not the L4 panel,
which produced four good hypotheses and missed this one entirely. **A proxy on the
wire, and reading what the model received.**

The panel was still worth holding: it found the 37% preactivation waste, it killed
two hypotheses (system-prompt rebuild, streaming) by reading opencode's source, and
Omar's correction stopped me sizing a fix against a fiction. But the decisive
evidence came from the wire, not from four L4 agents reasoning about source.

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
| 3 | Measure turn count | **DONE** — and it found the real bug. 25 turns → 5 calls. |
| 4 | Gate content routing on actual context pressure | **DONE** — `ycode 4604f07` |
| 5 | Gate distillation too (the half I missed) | **DONE** — `ycode 801e207` |
| 6 | Measure the *serialized schema bytes* per request | open — the proxy now reports it: ycode sends **10 tools / 6.2KB** per request, NOT 111. The router works. The "111 tools" theory is DEAD. |
| 7 | Audit every other place that trims/summarizes/excludes with no pressure check | **DONE — found FOUR more.** See below. `ycode de0ff1d` |
| 8 | Decide whether the tool-routing cascade should exist at all | **open — and the audit answered it.** See "34 lines". |
| — | ~~System-prompt rebuild~~ | **killed in meeting** — opencode does it too |
| — | ~~Streaming~~ | **killed in meeting** — both stream |
| — | ~~Go-vs-Node~~ | **killed in the brief** — identical on a trivial prompt |


---

# The audit — and the four it found

The instruction was: *"two identical bugs found; assume a third."* There were four.
A codex second opinion found one, a ratchet test found two, and **the user's question
found one** by refusing to accept a number I had not checked.

## 1. The budget was a global constant

Every consumer of the context machinery divided by the package-level
`CompactionThreshold` — a flat **100_000** — regardless of which model was on the
other end. `ContextBudgetForProvider` had computed the right numbers all along and
stored them on `Runtime.contextBudget`. **Nothing read them.**

For a model at or under 64K, all three layers land OUTSIDE the window:

| | fires at | usable on a 64K model |
|---|---|---|
| soft trim | 60,000 | 48,000 — **125%, never fires** |
| hard clear | 80,000 | 48,000 — **167%, never fires** |
| compaction | 100,000 | 48,000 — **208%, never fires** |

Dead code that reports *"context: healthy"* right up to the API error.

**And I got the model wrong.** I said deepseek was 64K. ycode's table says **128K** —
I had quoted the spec sheet instead of reading `api/capabilities.go`. So the
catastrophic version of this is a live TRAP, not a live FIRE. What is true for the
model we actually benchmark: **compaction is unreachable (100K vs 98K usable), and
trimming fires 3× too late** (60K where the correct line is 20.5K) — on a *non-caching*
provider, where every one of those tokens is re-billed at full price every single turn.

## 2. The reserve did not cover the reply it exists to reserve for

The user's question, and it was the right one: *"could it be a request parameter, not
a hard limit?"*

Yes — and following it exposed the bug. A 128K window **reserves 30,000** tokens while
`MaxOutputTokenCap` asks for up to **32,000**. Fill the usable window, request the
reply, and the request exceeds the window by the exact amount the reserve was supposed
to hold. Worse, on a 32K model we asked for a 32K reply — **the entire window**.

> You cannot reserve your way out of asking for too much. `max_tokens` is a request
> parameter; ask for less.

`max_tokens` is now derived from the window, and the reserve is derived from *it* —
one number, so the two cannot disagree.

## 3. The exemption list was inert — codex found this

`ExemptFromMasking` keys on `ContentBlock.Name`. **Neither tool-result construction
site ever set `Name`.** So the lookup was always false, the list protected nothing, and
every observation — `read_file` included — was maskable.

The unit test passed. It passed because **the test fixture set `Name`** and production
could not.

> A green test against a fixture production cannot produce is not evidence.

## 4. An unset budget read as a FULL WINDOW — the ratchet found this, inside my own fix

A zero-value `ContextBudget` has `SoftTrimAt() == 0`, so `chars/4 >= 0` is true for
**every** conversation including an empty one. A missing budget silently switched
content damage back on for everything.

**This is the original bug, hiding inside the fix for it.** Damaging the model's
observations is the aggressive act; it must never be reached by the ABSENCE of a
number.

## Also: "conservative"

An unknown model was assumed to have a **200K** window — the largest we support — and
the comment called that *"conservative default"*. It is the least conservative choice
available: point ycode at a local 8K model and it packs 100K tokens in. Guess low and
you pay for some avoidable compaction; guess high and nothing works. Unknown now means
**small**.

## The ratchet

`session/budget_reachable_test.go` fails the build if any threshold ever again sits
outside the window it claims to protect. **It found two of the four above on its first
run** — including one in the "correct" implementation I was about to trust: an 8K
window reserved 8,000 tokens, leaving **zero usable**.

---

# 34 lines

The user asked what claude / codex / opencode actually do. opencode's source is in
`priorart/`, so this is measured, not remembered.

| | window from | reserve | mechanisms | LOC |
|---|---|---|---|---|
| **opencode** | model registry (`model.limit.context`) | `min(20K, maxOutputTokens)` — **one** config knob | **ONE**: compact on overflow | **34** |
| **codex** | model config (`model_context_window()`) | — | compact; trims history only *"to fit context window"* | — |
| **ycode** | **a hardcoded switch table** | a magic table (8/16/30/40K) | **FIVE**: route · distill · mask · soft-trim · hard-clear · compact | **~600** |

opencode's entire context management is `session/overflow.ts`, and it is 34 lines:

```ts
const reserved = cfg.compaction?.reserved ?? Math.min(20_000, maxOutputTokens(model))
return Math.max(0, context - maxOutputTokens(model))     // usable
...
return count >= usable(input)                            // isOverflow
```

**That is the whole thing.** No routing. No distillation. No masking. No soft trim. No
hard clear. One question — *are you over?* — and one answer — *then compact.*

It also gates on `tokens.total`: **the real count the API reports back.** ycode
*estimates* at 4-chars-per-token. That is *why* ycode needs ratios and soft/hard tiers
— it does not trust its own numbers, so it hedges with layers, and every layer is a
place to be wrong.

**Five mechanisms, ~600 lines, and every one of them was broken.** I have spent this
work fixing bugs in code that should not exist.

## On making it configurable per role / band

The user asked whether window and reserve should be configurable per role
(steward/conductor/coder/tester) and per band (L1–L4).

**Nobody does this. Not one of the three.** And it is not an oversight — **band and
window are orthogonal.** A band is a capability peg for *routing*: which agent gets the
job. A window is a property of the *model*. An L1 coder on opus has 200K; an L4 steward
on a small local model has 8K. Sizing a context window by an agent's seniority is like
sizing a truck's fuel tank by the driver's job title.

What shipped is the knob opencode has, and no more:

- **`contextWindow`** — the window comes from a hardcoded table, and a table goes stale
  every time a provider ships a model. One number, no rebuild.
- **`contextReserved`** — opencode's `compaction.reserved`. The reserve is a
  consequence of the request *we* choose to make, so it is the one number an operator
  has standing to set.

## What this leaves

The strategic question (agenda item 8) is no longer a hunch. **Delete four of the five
mechanisms.** opencode is faster than ycode with 34 lines and none of them. Every bug
in this document is in machinery whose only justification was a 111-tool surface that
the proxy has already shown does not exist (10 tools, 6.2KB per request).

The next honest step is not another fix. It is a deletion.
