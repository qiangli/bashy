---
name: supervisor
description: >-
  Supervise a bashy sprint from an agent-tool session: observe and steer its
  designated manager without taking its lease or changing sprint state.
---

# Supervisor

A sprint is run by its **designated manager launched by bashy**. An open agent
tool is a supervisor, regardless of its model or capability. Launch the chosen
manager with `bashy sprint start N --owner AGENT --instruction "..."`.
An authorized `bashy sprint take N --owner AGENT` is a managed takeover, not a
shortcut for making your current session the manager.

Your sprint actions are **watch, tick, instruct, ping and emergency abort only**:

- Watch progress and use `bashy sprint tick N` for a read-only worksheet.
- Use `bashy sprint instruct N --instruction "..."` to steer the manager.
  Every instruction is recorded and counted as outside help beside its score.
- Use `bashy sprint ping N --body "..."` for attention; escalate unresolved
  decisions to the owner. Use `bashy sprint abort N` only for an emergency stop.

Do not assign, accept, edit, checkpoint, merge or end the sprint yourself.
An assigned worker may implement and commit its bounded story; that assignment
confers no manager authority. See `bashy skill show conductor` for the manager's
ladder workflow: `sprint assign/review/grade/merge/heat/arena`,
`accept --agent/--points/--rework`, `fail --blame ... --evidence`, and
`leaderboard --duty` / `leaderboard record`.

Bashy gives `BASHY_SPRINT_LEASE_TOKEN` only to the launched manager and binds it
to that agent. Never copy, print or borrow it. `BASHY_SPRINT_ENFORCE=should` is
guidance; `must` enforces the rule, with already-running sprints grandfathered.
Do not weaken enforcement. An explicit owner-authorized `--override --reason`
is logged on the sprint card and scorecard.

**Every commit needs `BASHY_AGENT` set to the actual assigned committing agent**,
plus `Sprint:`, `Story:` and `Story-ID:` trailers. Preserve the injected identity
and hooks. Missing attribution is a detected bypass: the sprint's **manage**
score automatically fails (0), and the bypassing agent is penalized.

Manager delivery scoring is **actual minus expected** for its team and stories.
Shipping everything is not sufficient evidence of good management; gates and
review must pass, and supervisor instructions remain part of the record.
