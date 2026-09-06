---
id: a785a2b46be4
kind: task
title: bashy sprint tick — expose the manager's turn worksheet as a subcommand for external tools
seq: 222
status: done
priority: p2
created: 2026-09-05T21:36:22.251639Z
assignee: claude-opus5-webconsole
sprint: 126
closed: 2026-09-05T21:55:13.348042Z
---

RECOMMENDATION, filed not built: this is a NEW VERB, and sprint 126 governs "a capability is out of scope no matter how small". Build it after the current MVP, or on an explicit call.

WHAT ALREADY EXISTS (checked, do not rebuild):
  `sprint take/start --owner NAME --watch` already stays attached and streams the
  unified inbox as NDJSON. The "constantly read your inbox" half of the loop is
  solved and is already machine-consumable.

WHAT DOES NOT EXIST:
  No verb answers the other half — "what changed since my last turn, and what
  needs a decision". An external tool must today run seven commands and diff the
  results itself, which means every harness reimplements the tick differently.

SHAPE:
  bashy sprint tick <id> [--as NAME] [--wait DUR] [--json]

  Returns ONE turn worksheet:
    - unread/directed mail counts (WITHOUT consuming cursors it should not)
    - board delta since this manager last ticked: stories added, changed, stalled
    - fleet readiness: the `weave fleet --auth` summary (installed is not signed in)
    - runs that produced NOTHING since the last tick  <- the progress-not-liveness
      signal; no existing command answers it, and it is the one that catches a
      worker that is alive and stuck
    - submissions awaiting review/merge
    - continuity age: is the brief stale
    - acceptance gate status: repeat, or wrap up

THE GOVERNING RULE — IT GATHERS, IT DOES NOT DECIDE.
  Prioritizing, assigning, merging and reassigning are judgement plus cost. The
  conductor skill already applies exactly this rule to its own bindings: PLAN,
  RESEARCH and RETRO stay unbound because "a command claiming to do them would be
  a lie". `tick` returns the inputs to a decision and never performs the act. A
  tick that auto-assigned would spend tokens on the manager behalf and hide the
  choice that matters.

MUST NOT:
  - refresh the lease as a side effect of being read. Reading mail refreshes the
    seat by design; a WORKSHEET must not, or polling it would forge liveness for
    a manager that has stopped working. This is the same hazard the board panel
    already refuses.
  - consume another name cursor. Same third-person PEEK rule as the inbox panel.
  - block unbounded. `--wait` is bounded like `bashy inbox --wait`; it returns
    early when something changes, which is what makes the loop a loop rather
    than a poll.

WHY IT IS WORTH BUILDING: the tick is now written down in two places
(`sprint --help` THE TICK, and the conductor skill) but every external tool has
to execute it by hand. A subcommand makes the loop portable across claude, codex,
agy and anything else, and makes the steps measurable — which is the only way to
tell "the manager skipped step 4" from "the manager had no way to see step 4".

SEPARATE, SMALLER: pkg/role.ProjectManager Title is still the string "project
manager", shared by meet and the sprint --owner flag help. The sprint surface was
renamed to "sprint manager"; that constant was left because it is structural and
shared. Reconcile it when meet vocabulary is next touched.
