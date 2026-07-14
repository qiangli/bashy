# `bashy pair`

*Shipped 2026-07-14. Replaces `bashy judge`, which could only ever talk.*

Two agents and a gate. One proposes, one **pairs** with it in a declared role, and a real
gate — a command, not a model — decides whether it is done.

```sh
bashy pair --role break --pair codex:gpt-5.5 "the budget accounting in budget.go"
```

---

## The one idea

Every "critic agent" design has the same hole: **a critic's finding is a claim, and someone
must now decide whether to believe it.** So you add a second critic to check the first, and
a third to break the tie, and you have built a courthouse.

Give the critic the keyboard and the whole regress collapses:

```
judge:  "this breaks on empty input"        -> a CLAIM.  Adjudicate it.
pair:   a test that FAILS on empty input    -> a PROOF.  The gate reads it.
```

- the test **fails** → the bug is real. Proven, by execution, for free.
- the test **passes** → the pair was wrong. Discarded. It cost tokens and nothing else.

**A finding that must be believed needs a judge. A finding that RUNS needs no one.**

That is why `pair` subsumes `judge` rather than sitting beside it. `judge` needed a *panel* —
three models voting — precisely because one opinion is unreliable and nothing could check it.
You do not need three models to vote on whether a bug is real when the bug is executable. You
need one exit code.

---

## The pair may never approve

Google Cloud's review-and-critique pattern terminates *"when the critic agent approves the
content."* That is the exact failure this repo has spent its life fighting:

> **all three harnesses exit 0 when they fail** — `docs/harness-ab-deepseek.md`
> …and then ycode did it too.
> …and then codex, asked to build a feature, shipped a **red test**, blamed the sandbox, and
> exited 0.

**Agency and authority are different axes,** and conflating them is how that keeps happening:

| | may act | may approve |
|---|---|---|
| **pair** | **YES** — edits, tests, fixes, runs | **never** |
| **gate** | no | **yes — it is the only thing that may** |

A real pair programmer can grab the keyboard and rewrite your function, and still cannot
declare the branch shippable. CI does that.

This also **removes** a shackle. `judge` was hardcoded read-only, for a good reason stated in
`chat.Options.ReadOnly`: *"a reviewer must not be able to modify what it reviews — an agent
with write access could fix the code and then approve its own fix."* Correct — **for a
judge.** The read-only rule was *compensation for the approval power*. Take the power away and
the shackle is no longer load-bearing, so the pair can safely have the keyboard.

One hole that does survive, and is closed explicitly: **an acting pair can edit files, and
`.bashy/gate` is a file.** So the gate definition is resolved **once, before the pair gets
write access**, and that same definition runs both times. The pair may change the code. It may
never change the ruler.

---

## The roles

`--role` is the generic axis. Declare one and you get a pair that plays it.

```
$ bashy pair --roles
ROLE             ACTS   SEES     EVIDENCE WHAT IT DOES
break            YES    yes      diff     WRITE the failing test that proves the bug
fix              YES    yes      diff     repair what you find, and prove the repair with a test
refute           no     yes      verdict  attack it in prose — cheap, and an unverified claim
second-opinion   YES    BLIND    diff     solve it independently, so convergence means something
validate         YES    yes      probe    check each claim by RUNNING something, not by reading
```

Three axes, and the contract is enforced at plan time — before a token is spent:

- **`acts`** — does it get the keyboard? A role that cannot act cannot produce a diff, only a
  description of one. Declaring `evidence: diff` with `acts: false` is a validation error.
- **`sees_proposal`** — see below.
- **`authority`** — `reject` or `advise`. **Never `approve`**; `Validate()` refuses it.
  A `reject`-authority role with **no gate** is refused outright: without one, the *model*
  becomes the arbiter of done by default, because nothing else is.

**Evidence is ranked: `diff` > `probe` > `verdict`** — the same ladder as the
fleet-evidence-invariant. A diff can be RUN, so it proves itself. A verdict is prose.

Only **one** builtin (`refute`) is a pure commentator, and it is the one to reach for last.
A test enforces that: *"a pair that only talks is a judge, and judge is what this replaces."*

### The blind one

`second-opinion` is the only role that does **not** see the proposal, and that is the entire
content of it.

> **A "second opinion" that reads your answer first is not a second opinion. It is a review.**

Anchoring destroys the independence you are paying for. Measured: in a four-L4 design meeting
the hypothesis was stated up front, three of the four **agreed**, and all three were wrong.
The one that disagreed was the only one worth the tokens.

A blind pair cannot *agree* with you. It can only **converge** with you — and convergence is
evidence in a way agreement never is.

`Validate()` refuses a sighted `second-opinion`. The name would be a lie.

---

## What a run proves

The pair runs the gate **twice**, and the baseline is not ceremony:

> **You cannot attribute a red gate to the pair unless you know it was green before.**

Concluding "the pair found a bug" from a red gate you never saw green is a finding reached by
the **absence of evidence** — the exact defect class catalogued in `docs/absence-of-evidence.md`.
Committing it *in the tool built to catch it* would be a bad joke.

| before | after | outcome | exit |
|---|---|---|---|
| green | **RED** | **`proved`** — the pair wrote something that runs, and running it exposed a real defect | **4** |
| RED | green | `repaired` | 0 |
| green | green | `held` — attacked it, found nothing the gate can see. A real, cheap result | 0 |
| RED | RED | `broken-before` — a second failure on top of a failure is not a signal | 5 |
| — | — | `ungated` — nothing checked the claim (advise roles only) | 0 |

**Exit codes are distinct because a conductor must react differently.** `4` means *do not
retry* — retrying a proof just re-proves it; send it back to the proposer with the failing test
attached. `1` means the plumbing broke, and retry is legitimate. Collapsing them into "nonzero"
is how a conductor retries a proof nine times.

The gate runs **regardless of what the pair said** — not "if the pair approved" (it cannot),
and not "unless the pair objected" either. An objection is a claim to check, not a proven
defect, and skipping the gate on a model's say-so hands it back the authority this whole thing
takes away.

---

## Model diversity

The pair should be a **different model family** than the proposer. `pair` warns, loudly, when
it is not:

> *proposer and pair are both from the "opus" family — they share a blind spot, so agreement
> between them is not a second signal, it is the same signal louder.*

It warns rather than refuses: a same-family pair is degraded, not useless, and an operator may
have exactly one provider. **An agent may never pair with itself** — that one is an error. It
will agree with the reasoning it just produced, because it *is* the reasoning it just produced.

---

## Verified, end to end

A scratch repo with a planted bug of exactly the shape this tool claims to catch — one the
existing test suite **passes** on:

```go
func WithinBudget(total int, usage []Usage) bool {
	return Remaining(total, usage) > 0   // Usage.Reported is never read.
}                                        // No usage reported => "nothing spent" => "fine".
```

`go test ./...` is **green**. The bug is still there. Then:

```sh
$ bashy pair --role break --pair codex:gpt-5.5 "budget accounting; Usage.Reported says
    whether the provider actually reported token usage"

PROVED — codex:gpt-5.5 (break) wrote a failing test against green.
The defect is real; the gate says so, not the model.

  gate      before=green  after=RED
$ echo $?
4
```

What it left behind — six lines, touching **only** the test file, not the source and not the
gate:

```go
func TestWithinBudgetFailsClosedWhenUsageWasNotReported(t *testing.T) {
	if WithinBudget(1000, []Usage{{Reported: false}}) {
		t.Error("should not be within budget when provider usage was not reported")
	}
}
```

It named the test after the bug class. **Nobody adjudicated this.** The gate did.

---

## Usage

```sh
# attack work that already exists (the common case — what `judge --diff` did)
bashy pair --role break --pair codex:gpt-5.5 "what this change is supposed to do"

# greenfield: one writes it, the other breaks it
bashy pair --role break --proposer claude:opus4.8 --pair codex:gpt-5.5 "add a CSV parser"

# blind independent solve, for a hard design call
bashy pair --role second-opinion --pair ycode:glm-5.2 "should the cache key include the locale?"

bashy pair --roles          # what each role does and what it produces
bashy pair --json           # bashy-pair-v1
```

An acting pair needs write authority in its workspace
(`BASHY_ALLOW_UNSAFE_AGENT_LAUNCH=1`, or run it inside `bashy weave` where the isolation is
structural). A `refute` pair does not — it never touches the tree.

## `judge` is still its own verb

**`pair` did not replace `judge`. Both exist, independently.** `bashy judge` still runs the
original `pkg/judge` panel — N reviewers, combined verdict, unchanged. Nothing was aliased and
nothing was deleted.

This is worth stating plainly because the commits that introduced `pair`
(coreutils `36f9ed9`, bashy `da681ec`) claim otherwise. They say *"judge stays as an alias"*
and *"net new verbs: ZERO."* **Both are false** — the alias was described and never wired.
An assertion made in the commit message of the very tool built to catch assertions made
without evidence. It is left in the history rather than force-pushed over, because a
correction you can read is worth more than a history that looks clean.

The verb count went **+1**, deliberately, and stays there for now:

| verb | runs | when to reach for it |
|---|---|---|
| **`pair`** | `pkg/pair` | **default.** The pair acts; its finding is executable, so the gate settles it |
| `judge` | `pkg/judge` | a panel verdict on something with no gate to run — a plan, a design, prose |

They genuinely do different jobs, which is the honest reason to keep both: `judge` can review a
thing that cannot be executed. `pair` cannot — its whole value is that the gate can run what
the pair wrote. Collapsing `judge` into `pair --role refute` is possible (that role is exactly
judge-without-the-panel) and may happen later; it would mean repointing weave's in-process
`judge.RunReader`/`RunRecorder` hooks. Not today.
