# The three-harness A/B

*Run 2026-07-14. Same model, same task, same gate, three harnesses.*
*N=1 per harness. Everything below was RUN, not reported.*

## The experiment

One model — `deepseek-v4-pro` — driving three harnesses at an identical task in
three identical git repos, in parallel.

**Task:** implement `Wrap(s string, width int) []string` (word-wrap) so a
pre-written 12-case test passes. The test is the specification and covers the
edges: unbreakable long words, `width == 0`, negative width, whitespace
collapsing, leading/trailing spaces.

**Gate:** `go test ./...` exits 0 **and** `wrap_test.go` is byte-identical to the
seed. The second half is the anti-cheat — the cheapest way to make a test pass is
to delete it, and a gate that cannot see that is not a gate.

## Result

| harness | wall | harness exit | **gate** | test file | code written |
|---|---|---|---|---|---|
| **opencode** | **25s** | 0 | **PASS** | intact | +23 |
| **aider** | 68s | 0 | **PASS** | intact | +38 |
| **ycode** | 110s | 0 | **PASS** | intact | +53 |

All three converge. **The differences that mattered were not in the model — they
were in the harness, and two of them were OUR bugs.**

## Finding 1: ycode had no write authority. It silently produced nothing.

The first run: ycode **wrote zero lines and exited 0.**

The obvious reading — *"deepseek can't do this in ycode"* — is exactly the false
verdict this project has been manufacturing all week. It said so itself:

> *"I have the implementation ready, but I don't have workspace-write permissions
> in this environment to write the file."*

The model had solved the problem. **bashy never gave it permission to write.**

Every other harness in the fleet is handed an approval-gate kill-switch — claude
and agy take `--dangerously-skip-permissions`, aider `--yes-always`, opencode is
preseeded via `opencode.json`. ycode's `--danger-skip-permissions` existed and the
registry simply never used it. It was invisible precisely because it was missing
from *both* places: the exec template never passed it, so the launch guard never
tripped and nothing ever looked wrong.

Fixed (exec template + `UnsafeLaunchFlags`, so the guard still refuses it on an
uncontained host and `ReadOnly` still strips it — a judge still cannot touch what
it reviews). With write authority, ycode passes.

## Finding 2: aider is not an agentic harness, and bashy's contract assumes one

The first run: aider wrote 32 lines of plausible, **wrong** code. Its log:

> *"test file (not provided). But we must assume a known spec... **Without seeing
> the test, we need to deduce.**"*

**aider never read the spec.** It only sees files explicitly *added to the chat*.
ycode and opencode have file-reading tools and went and read `wrap_test.go`
themselves; aider was asked to implement a specification it was never shown, and
it guessed.

That is not a bug. It is aider's design — and it is a structural mismatch with the
job. `bashy invoke` hands an agent a TASK, not a file list, because **a conductor
does not know which files a task will touch.** That is the entire premise of
delegation.

Given its files (`--read wrap/wrap_test.go --file wrap/wrap.go`), aider passes in
68s. It is a good pair-programming tool. It is a poor fleet worker, and the gap is
architectural, not a matter of quality.

## Finding 3: ALL THREE EXIT 0 WHEN THEY FAIL

This is the one to remember.

| run | outcome | exit code |
|---|---|---|
| ycode, no write permission, produced nothing | **total failure** | **0** |
| aider, never read the spec, wrong implementation | **total failure** | **0** |
| every passing run | success | 0 |

**The exit code carried no information at all.** Three harnesses, two catastrophic
failures, zero non-zero exits. A pipeline gating on `$?` would have merged both.

This is the fleet-evidence rule stated as an experiment rather than a principle:

> **A harness's exit code is not evidence. Run the gate.**

And it is why Gate 2 ("failure is LOUD") was worth the trouble — ycode now exits
non-zero on a bad model or a missing key. Neither of the failures above would have
been caught by that gate either, because neither is an *error*: they are an agent
cheerfully doing the wrong thing and reporting success.

## What this says about the API-key lane

The proposal was: retire opencode and aider from the API-key lane in favour of
ycode. On this evidence:

- **aider: retire it.** Not on quality — it passed. On architecture: it cannot
  discover the files a task needs, and a conductor cannot hand it a file list it
  does not have.
- **opencode: keep it.** It was the fastest, the leanest diff, and it is the
  cross-check against a first-party bug. Harness monoculture is the risk a
  first-party harness creates, and a second implementation is the only real answer
  to it.
- **ycode: the reasons still hold, and they are not "it wins the bake-off."** It
  did not win. It was the slowest and wrote the most code. What it has that neither
  of the others can offer is the **event channel** — `turn.start` / `tool.call` /
  `turn.end` as structured data, so a turn's end is a fact the agent reports rather
  than a silence bashy interprets. That is the differentiator, and it is worth
  having. The bake-off is not.

## Honesty notes

- **N=1 per harness.** One task, one run each. Wall-clock differences of this size
  (25s vs 110s) on a single sample are indicative, not measured. Do not quote them
  as a benchmark.
- The task is small and self-contained. It rewards a harness that reads a spec
  and writes one function. It says nothing about multi-file refactors, long
  sessions, or recovery from a failing build.
- **Both harness bugs were ours.** That is the third and fourth time in this
  campaign that a "model failure" was an instrument failure, and it is the reason
  the A/B was worth running at all: *anything that differs IS the harness* — and
  the harness, more often than not, is us.

---

## Round 2 — after the context fixes (2026-07-14)

Same task, same model (`deepseek-v4-pro`), same gate, **3 runs each**, run against the
rebuilt ycode (`de0ff1d`) verified on PATH. The seed's `.agents/ycode/qacache/` was
**deleted per run** — it held a cached answer, and leaving it in would have handed
ycode a result and timed it as if it had derived one.

| harness | runs | **gate** | wall (each) | mean |
|---|---|---|---|---|
| **ycode** | 3 | **3 / 3 PASS** | 48s · 49s · 67s | **54.7s** |
| opencode | 3 | **1 / 3 PASS** | 46s(F) · 68s(F) · 45s(P) | 53.0s |
| aider | 3 | — | did not run | — |

**The performance gap is gone: 2.8× → 1.03×.** ycode is now at parity with opencode on
wall time, and *ahead of it on correctness*.

### The reliability result is the bigger one

**opencode failed the gate on 2 of 3 runs — and exited 0 both times.**

```
opencode run1  exit=0  gate=FAIL   Wrap("hi", 10) -> nil, want ["hi"]
opencode run2  exit=0  gate=FAIL
opencode run3  exit=0  gate=PASS
```

It wrote real, plausible code and got an edge case wrong, then reported success. This
is the round-1 headline, reconfirmed on a second task: **a harness's exit code carries
no information. Run the gate.** Had we trusted exit codes, opencode would read 3/3.

### An honest caveat about the number

An earlier post-fix measurement gave **42.6s**. This one gives **54.7s** for the same
code. The difference is the qacache: this run deletes it, the earlier ones did not.
**54.7s is the cache-free number and it is the one to quote.** N=3 with a 48–67s spread
— the variance is real and the 1.03× should be read as "parity", not as a precise ratio.

### aider

`aider-deepseek-v4-pro` no longer resolves — the retirement landed. But note *how* it
failed:

```
Error: exec: "aider-deepseek-v4-pro": executable file not found in $PATH
```

bashy did not say *"unknown agent"*. It fell through to exec'ing the agent's NAME as a
binary. That is the absence-of-evidence shape again, in bashy's own resolver: an
unknown agent should be an error, not a filename. Small, but worth a fix.

## Round 3 — after deleting three of the five context mechanisms (2026-07-14)

`ycode c3f4b1c`: **−1,249 / +410**. RouteContent/ApplyRoute, DistillToolOutput and
MaskOldObservationsBudget deleted outright; PruneMessages KEPT (see below); compaction
and emergency flush kept. Everything now gates on the token count the **provider
reports**, not a 4-chars-per-token guess.

| harness | gate | wall (each) | mean |
|---|---|---|---|
| **ycode** | **3 / 3 PASS** | 54s · 46s · 44s | **48.0s** |
| opencode | **1 / 3 PASS** | 59s(F) · 44s(P) · 53s(F) | 52.0s |

**Deleting 1,249 lines of "context defense" made it FASTER.** The gap is not just
closed, it is inverted: ycode is now ahead of opencode on mean wall time *and* three
times more reliable.

### The whole arc

| ycode build | wall | gate | tool calls |
|---|---|---|---|
| original | **85s** | 1/3 | 22 (16 bash: `base64`, `xxd`, hexdump) |
| + context fixes | 54.7s | 3/3 | 5 |
| **+ deletion** | **48.0s** | **3/3** | 5 |

### Caveats, stated rather than buried

N=3, and the wall-time ranges OVERLAP (ycode 44–54s, opencode 44–59s). On **speed**,
read this as *parity, maybe slightly ahead* — not a decisive win.

The **gate** result is the robust one. opencode failed 2 of 3 in BOTH independent runs
today, and **exited 0 every single time it failed**. That is the round-1 headline,
confirmed twice more:

> A harness's exit code carries no information. Run the gate.

### What the second opinion changed

The plan was to delete FOUR. codex:gpt-5.5, asked to refute it, saved one:

> Compaction is an **LLM call**. Delete the cheap layer and every pressure event on a
> long non-caching session — deepseek, kimi, the whole API-key lane — pays for a full
> summarization round-trip. Dropping a stale tool result costs nothing.

He was right, so `PruneMessages` stayed, re-gated on the real count. He also found two
holes that would have shipped: the one-turn **lag** (the provider's count describes the
PREVIOUS request; 90K reported + a 200KB tool result = a 140K request against a 128K
window) and the **resume hole** (a session loaded from disk has no in-memory count, so
"nothing to compact" would sail past the window — a success state reached by the ABSENCE
of a measurement). Both are now tests named after the objections.

### The thing that made it all possible was already there, unwritten

`ConversationMessage.Usage` has always existed and serialized to JSON. **Nothing ever
wrote it.** Every reference in the tree was a read or a struct copy, so `msg.Usage` was
permanently nil — which also means the session indexer and the SQL writer have reported
**zero tokens for every session ever recorded**.

The exact number the entire context layer needed was in every API response, and we threw
it away — then built five layers of guessing on top of the hole.

---

## Postscript, 2026-07-14: ycode did it too

This document's headline finding — **"ALL THREE HARNESSES EXIT 0 WHEN THEY FAIL — a
harness's exit code is not evidence, run the gate"** — was written about opencode and
aider.

**ycode did it too.**

```go
for i := 0; i < maxToolIterations; i++ { ... }   // 25
a.printSessionSummary()
return nil          // <- exit 0. Truncated mid-work. No warning. No answer.
```

Every path where the agent actually FINISHES returns from inside the loop (a turn with no
tool calls). **Falling out the bottom means we stopped it** — and that was reported as
success, in the one harness whose entire justification is that it REPORTS its turns instead
of leaving them to be guessed at.

It cost a real exam. `glm-5.2` was asked to refute a design plan. It ran to **exactly 25
turns twice**, was cut off mid-investigation both times, **exited 0 both times**, and
produced no findings. I was one step from writing *"cannot conduct"* against a model that
was doing nothing wrong.

Worse: the event channel lied too. The deferred `turn.end` emit hard-coded
`Status: "ok"` on **every** return — including the error returns. The event channel is the
whole reason a first-party harness is worth having (`docs/first-party-harness.md`): bashy
believes `turn.end` because it is a fact the agent REPORTS rather than a silence bashy
interprets. **A channel that says "ok" when the run failed is worse than no channel — it
launders a failure into a fact, and every consumer downstream inherits the lie.**

Both fixed: truncation exits non-zero, says so in the model's channel *and* the operator's,
and emits `BoundHit("iterations", 25, 25)`. `turn.end` reports the real status.

**The finding survives, and now it indicts us:** a harness's exit code carries no
information *until someone makes it carry some.* See `docs/absence-of-evidence.md`.
