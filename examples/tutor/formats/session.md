---
name: session
roles:
- owner
- participant
audience:
minutes: 90
phases:
- warm-up 10
- work 60
- demo 15
- retro 5
default_step_minutes: 20
owner_step_minutes: 5
default_done_when: the outcome of this step is shown to another person and noted on the card
nudge_points:
- halfway
- 2m-left
pairing: pairs of two, rotate at every phase boundary; a lone participant pairs with the owner
desk_tools:
- warm-up: board, notes
- work: editor, terminal, tests
- demo: display, timer
- retro: notes
tone: Ask before you tell. A hint is a question or the next smallest step, never the answer or the code.
owner_steps:
- prepare: content ready, desks and tools checked, pairs decided
- facilitate: clock running, nudges sent at the format's nudge points
- review: evidence collected per step, retro notes written on the card
---

# Session — guidance for the planning agent

This is the default session format: one owner, a handful of participants, one
goal, one clock. Copy this file to make your own format and pass it with
`--format <name-or-path>`.

Shape each step around a visible outcome:

- Every participant step is something a person can finish inside its timebox and
  show to somebody. A step with no visible outcome is not a step.
- `done_when` is the evidence, phrased so another person can check it without
  asking the author ("the test for X passes", "three examples on the board").
- Keep the phase order of the format. `warm-up` recalls what is already known,
  `work` is where the goal is actually attempted, `demo` is where participants show
  each other, `retro` is where the session names what to carry forward.
- Owners come from `--people`, round-robin, so nobody idles. With no people,
  leave the owner empty and the owner assigns at the desk.
- The owner's own steps (prepare / facilitate / review) belong on the card
  too; they are what makes the session repeatable next week.
- Honour the tone: the session is timeboxed so participants can be stuck safely.
  Hints stay hints.
