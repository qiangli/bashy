---
id: 39748bebea4e
kind: task
title: 'RETRO: the agent that used bashy sprint reviews it — friction found while sitting the seat'
seq: 226
status: done
priority: p1
created: 2026-09-06T00:58:22.708201Z
sprint: 127
closed: 2026-09-06T01:03:58.683029Z
---

A RETRO BY THE AGENT THAT ACTUALLY USED IT, not a design review. Read the code
(coreutils/pkg/weave, bashy/internal/agentos) AND recall the lived experience of taking
sprint 126, managing it, and handing it over — then say what should change.

THE RULE THAT SEPARATES A RETRO FROM A WISH LIST: every finding must cite the MOMENT it
cost something — a command that failed, a wrong answer believed, a step done twice, a
guess that was wrong. "It would be nice if" is not a finding. If nothing was lost, it
does not go in the list.

SEED OBSERVATIONS FROM THE SESSION OF 2026-09-05/06 (this is evidence, not the answer —
verify each, and expect to find more the seat has not hit yet):

VERIFIED, 2026-09-06, by probe:
  1. `todo add --sprint N` ASSOCIATES CORRECTLY, but two READ paths hide it, so the
     writer cannot confirm their own write:
       - `todo show <id>` does not display the sprint field at all;
       - `todo list --sprint N` returns NOTHING for an item that IS on sprint N.
     The sprint BOARD sees the item, so the data is right and the readers lie. Measured
     cost: believed the add had failed, ran a redundant `todo edit --sprint`, and only
     found the truth by grepping the board. A write you cannot read back is
     indistinguishable from a write that did not happen.

OBSERVED WHILE WORKING (verify before acting):
  2. VERB DISCOVERY IS GUESSWORK ACROSS THE FAMILY. Wrong guesses made this session:
     `sprint plan --add` (no such verb; the answer is `sprint goal add --story`),
     `sprint goal add --key` (it is `--id`), `todo edit --status` (it is `todo start` /
     `todo status`), `weave show` (it is `weave status` — while `sprint show` exists).
     Each cost a round trip. `show` existing on one noun and not its sibling is the
     sharpest case.
  3. EVERY INVOCATION EMITS NOISE ON STDERR — a telemetry line and a hint JSON — so
     nearly every captured command in this session needed `grep -v telemetry`. For a
     human that is clutter; for an AGENT parsing output it is a correctness hazard, and
     it costs tokens on every single call. Ask whether the hint engine should be silent
     when stdout is not a TTY, or when --json is set.
  4. ASSIGNING A STORY TO THE SEAT I HOLD FAILED TO NOTIFY: "claude-opus5-webconsole not
     notified: authored communication: unattributed agent session". The lease was held
     under that exact name at that exact moment. Worth understanding: is holding a
     sprint lease not an identity claim? If it is not, should it be?
  5. `sprint show` output is dominated by the acceptance text. It is the right content
     for a takeover and the wrong content for the tenth read of a shift. `sprint tick`
     now covers the recurring read; check whether `show` should have a short form.
  6. TWO FUNCTIONS ANSWERED "who is writing this" (weaveConductorName vs
     weaveStoryConductorName) and the wrong one was wired into checkpoint — fixed in
     ff01bbb3. THE RETRO QUESTION IS NOT THE BUG, IT IS THE SHAPE: look for other places
     where two helpers answer one question, because that is where the next one is.

METHOD:
  - Walk the seat lifecycle in order (take -> tick -> assign -> monitor -> review ->
    checkpoint -> handoff) and at each step ask: what did I have to already know that
    the tool did not tell me?
  - Read the refusal messages specifically. A refusal is the tool teaching, and this
    sprint has already fixed three that taught nothing or taught the wrong thing.
  - Separate DEFECTS (it lied, it failed, it cost a round trip) from ERGONOMICS (it was
    fine but slow) and rank by measured cost. Do not merge the two lists.

DELIVERABLE: a findings list, each with its cost, split defects/ergonomics, filed as
individual stories where the fix is clear. Not a document nobody reads — the output is
FILED WORK. Fixes are out of scope for this story except where a one-line fix is
obviously right and cheaper to make than to describe.
