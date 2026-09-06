---
id: 5f59a8f844f7
kind: task
title: 'P1: verb asymmetry across sprint/weave/todo makes every command a guess'
seq: 229
status: todo
priority: p1
created: 2026-09-06T01:03:37.197207Z
sprint: 127
---

MEASURED matrix across the three sibling nouns:

    noun     show  comment  status  list
    sprint    Y      Y        Y      N
    weave     N      Y        Y      Y
    todo      Y      N        Y      Y

Each noun is missing a DIFFERENT verb; only `status` is universal. There is NO RULE TO
LEARN, so an agent cannot generalize from one noun to its sibling — every combination is
memorized or guessed. Four wrong guesses in one session: `sprint plan --add` (it is
`sprint goal add --story`), `sprint goal add --key` (it is `--id`), `todo edit --status`
(it is `todo start`), `weave show` (it is `weave status`, while `sprint show` exists).

COST IS A ROUND TRIP EACH, every time, for every agent, forever. That is the argument for
fixing it rather than documenting it.

DECIDE FIRST, then implement — the answer is not obviously "add all twelve":
  * Some absences are RIGHT. `sprint list` may be deliberate (the board IS the list).
    `weave show` may be deliberate if `status` is the richer verb. Establish intent
    before adding anything; an alias added against a deliberate omission is worse than
    the gap.
  * For each genuine gap, the cheapest honest fix may be an ALIAS or a NAMED REFUSAL
    ("weave has no `show` — use `weave status`, which reports reconciled state"), not a
    new implementation. A refusal that names the right verb costs one line and ends the
    guessing.

NON-NEGOTIABLE: whatever is decided, it must be DISCOVERABLE without running the wrong
command first. That is the whole finding.
