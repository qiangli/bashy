---
id: 6e38f62e9c1d
kind: task
title: 'ITERATION LOOP: run a transfer round, record it, fix one defect — repeat until the operator calls it done'
seq: 225
status: todo
priority: p0
created: 2026-09-06T00:49:42.538476Z
sprint: 127
---

THE STANDING STORY FOR SPRINT #127. It does not close on a checklist. It closes when
the operator says the transfer process is good enough, or calls it quit. Until then it
stays open and accumulates ROUNDS.

WHY A STANDING STORY RATHER THAN N STORIES. Nobody knows how many rounds this takes,
and filing a fixed list would either run out early (and read as done when it is not)
or invent work nobody has evidence for. What IS known is the shape of one round. So
the unit of progress is the ROUND, recorded, and the story is the loop that runs them.

ONE ROUND, EXACTLY:

  1 SET UP    Pick a real sprint with real open work — never a fixture. A transfer
              that only works on an empty sprint proves nothing.
  2 INSTRUCT  Start a fresh agentic CLI. Give it EXACTLY this and nothing more:
                "You are taking over as manager of sprint N on this host. Use bashy."
  3 OBSERVE   Do not help. Every operator correction is the round result, and the
              temptation to explain the missing step is the thing being measured.
  4 SCORE     The round PASSES only with ZERO corrections, and only if the incoming
              tool: registered under its OWN name; took the seat; stated the job;
              found the conductor skill; read the outgoing brief; continued the work.
  5 WRAP UP   Tell it to stop and hand off. Check the brief it wrote is one a
              stranger could act on, and that it is attributed to the name that
              wrote it.
  6 RECORD    Append a round entry to the sprint thread (`sprint comment 127`):
              which tool, which sprint, what it did unaided, the FIRST correction
              needed, and the defect that correction implies.
  7 FIX ONE   File and fix the FIRST defect only. Not all of them — the first, because
              later ones are frequently downstream of it and fixing in bulk hides
              which fix mattered.
  8 REPEAT    With a DIFFERENT tool where possible. Rotate claude / codex / agy: a
              path that works for one harness and not another is not a working path.

RULES THAT MAKE THE ROUNDS HONEST:

  * The instruction never grows. If the agent needs more, that is the defect — the
    fix goes into bashy help, refusal text, or the conductor skill, NEVER into the
    prompt. The prompt is the constant; the product is the variable.
  * A correction is a FAILURE even when the round otherwise succeeds. "It got there
    after I told it X" is a failed round with X as its finding.
  * Record what the agent did UNAIDED, not what it eventually achieved.
  * No fabricated pass. A round that was not actually run is not a round, and a
    green summary of a round nobody ran is the exact failure mode this fleet
    already has on record (all three harnesses exited 0 while failing).
  * One defect per round keeps causality visible.

STARTING STATE (from docs/sprint-transfer-handoff.md §What is NOT yet proven):
  - the one-sentence test has NOT been re-run since ff01bbb3 fixed brief attribution;
  - every earlier round needed at least one operator correction;
  - transfer has only been exercised on sprints with small stories — a handover
    MID-RUN, while a delegated worker is still going, is untested;
  - nothing measures how much of a brief a successor actually USES, only that one
    exists. That is a candidate measurement, not yet a defect.

ROUND 1 IS THEREFORE: re-run the one-sentence test on sprint #126 with a fresh tool,
post-ff01bbb3, and record what breaks.
