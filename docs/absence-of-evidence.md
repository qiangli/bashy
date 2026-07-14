# A success state reached by the absence of evidence

*2026-07-14. Seven instances in one day, in one codebase, found while fixing each other.*

This is not a bug. It is a **shape**, and once you can see it you find it everywhere.

> **A record written by a process that did not survive is not evidence of absence.**
> **If the artifact is on disk, ask the artifact.**

---

## The seven

Each of these was declared, reviewed, merged, and **dead** — and each produced a
*plausible answer that was not true*.

| # | what it was | what it did |
|---|---|---|
| 1 | `ConversationMessage.Usage` | declared, serialized to JSON, **never written**. The session indexer and SQL writer have reported **zero tokens for every session ever recorded**. |
| 2 | `ExemptFromMasking` | keyed on `ContentBlock.Name` — which **production never set**. The exemption list protected nothing. Its unit test passed because *the test fixture* set the field. |
| 3 | `StreamOptions` | declared, documented *"enables usage reporting"*, **never populated**. |
| 4 | `SessionTotalCost` | declared, created by the meter, **never recorded**. |
| 5 | 3 config fields | `maxToolIterations`, `contextWindow`, `contextReserved` — parsed, validated, **silently discarded** by a hand-written merge. Setting them did nothing. No error. |
| 6 | the 25-iteration cap | cut an agent off mid-investigation and **exited 0**. |
| 7 | the pricing fallback | an unknown model billed at **Claude's rate** — a confident, specific, **wrong** number. GLM reported at **5.5× its real cost**. |

## Why they are the same bug

Every one is a **decision made from a label instead of an artifact**:

- *"The field exists, so the value is there."* → it was `nil`.
- *"The test passes, so the code works."* → the test took a path production doesn't.
- *"It exited 0, so it succeeded."* → it was cut off mid-work.
- *"The number is 6482, so we know the size."* → we guessed it.
- *"Cost is $0.45, so it's expensive."* → we'd never heard of the model.

**The absence of a failure signal was read as the presence of success.**

## The three that cost the most

**The model was blamed for the harness.** Four separate times, a harness bug nearly got
recorded as a *fact about a model* — a lie that would have outlived the evidence:

- a 4096-byte pty truncation (`MAX_CANON`) presented as *"deepseek did nothing"*
- a 25-turn cap presented as *"glm cannot conduct"*
- a subagent cap discarding findings, presented as *"its delegates fail"*
- three recovered 429s presented as *"rate limits killed the run"*

**A number without its provenance cannot be audited.** The *only* bug caught by a signal
was caught by a log line printing `from_provider=false` **next to the token count**. As a
log line it took a human staring at stderr. As a span attribute it is a query.

**A bound you cannot see is not a bound, it is a trap.** Every cap in the system bound
silently — and the ones that *recover* are worse than the ones that fail, because nobody
investigates them until they stop recovering.

## What was built in response

Not more tests. **Two primitives**, in `coreutils/pkg/telemetry`:

```go
Provenance(ctx, "context.tokens", 6482, "provider")   // the number, and WHERE IT CAME FROM
BoundHit(ctx, "iterations", 25, 25, "not finished")   // a limit records when it BINDS
```

`6482` tells you nothing. `6482 from the provider` says the gate is running on fact.
`6482 estimated` says it is running on a guess. **The difference is the entire bug.**

## And the tools lied too

Worth its own section, because it happened **four times in one day** and the lesson is not
about this codebase:

- `go build … | head -3 && echo "BUILD OK"` — the `&&` chains off **`head`'s** exit code. Reported success on a **failed build**. Twice.
- `rm -f /tmp/spans.jsonl` while the receiver held it open — writes went to a **deleted inode**, and I concluded *"bashy doesn't emit"* from my own broken instrument. Twice.
- `pgrep -f "model glm-5.2"` returned 0 for a **running process** — bad pattern, read as "it's dead."
- The umbrella's OTLP receiver **silently dropped span events** — so telemetry that worked perfectly showed nothing, and the obvious conclusion was that the code was broken.

> **An instrument that silently discards a signal is worse than no instrument: it produces
> confident negatives.**

## The rule

1. **Ask the artifact, not the label.** The file, the exit code, the wire, the span.
2. **A test that takes a different path from production tests a different program.** A mock proves the emitter was *called*, not that the data *arrived*.
3. **A declared field that nothing writes is a lie with a type signature.** Grep for fields whose only references are copies.
4. **Every limit says when it binds. Especially when the run recovers.**
5. **Every number used in a decision carries where it came from.**
6. **Two constructors of one thing is one more than can be kept in step.** ycode had *two* `interp.New()` sites; wiring one produced exactly the symptom of a broken middleware — linked, correct, never firing.

## Related

- `docs/fleet-evidence-invariant.md` (umbrella) — the same law at fleet scale: *no state that asserts success may be reached by the ABSENCE of evidence.* This document is that invariant, found again from the bottom up, inside one process.
- `docs/harness-ab-deepseek.md` — *"all three harnesses exit 0 when they fail."* Written about opencode and aider. **ycode did it too.**
