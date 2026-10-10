---
name: sprint
description: Start or direct a Bashy sprint while requiring an explicit canonical project manager and managed delivery. Use when the user invokes /sprint or asks an agent to create, start, or direct a sprint.
---

# Sprint

Translate the user's request into Bashy sprint commands. This skill is the
agent-facing `/sprint` adapter; do not build or invoke a second prompt parser.

## Default maintenance sprint — #9999

Use #9999 for a quick fix or an urgent task unrelated to the current sprint.
Commands that address one sprint default to #9999 when its operand is omitted;
explicit sprint operands always take precedence. `bashy sprint` still shows
the whole board, and `sprint add` creates a new ordinary numbered sprint.

Repository `bashy todo add "fix something"` defaults to #9999 and tracks the
repository automatically. Use `--sprint N` for another sprint or `--no-sprint`
for an unlinked todo. Personal todos remain unlinked by default; maintenance
stories use a repository store (`--repo` or `--base-dir`). Zero still means
unlinked, including `todo edit ID --sprint 0`.

#9999 is reserved and created on first selection; ordinary numbering skips it
without jumping ahead. It follows the same lifecycle and authorization rules
as other sprints. Read its card and the story, keep an active manager when
coordinating work, and move backlog/done to doing before claiming. Never assume
filing a story reopens the sprint or appoints a manager. Its built-in brief
provides the initial goal/plan; each fix's story carries scope and acceptance.

```text
bashy todo add "fix something" --note "Problem, expected behavior, verification"
bashy sprint show
bashy sprint take --owner YOUR_REGISTERED_AGENT
bashy sprint move doing
bashy sprint claim STORY_ID --owner YOUR_REGISTERED_AGENT
bashy sprint submit STORY_ID --as YOUR_REGISTERED_AGENT -m "delivery evidence"
bashy sprint accept STORY_ID -m "manager verification"
bashy sprint handoff -m "completed work and remaining stories"
```

Reuse the existing manager instead of taking an occupied lease. Acceptance is
performed by the current manager under the normal lease checks. For a comment
without a sprint operand, use `sprint comment -m "text"`. Start, stop, end and
advance work normally; no recurring flag is needed for an individual fix.
Delivery commits carry `Sprint: #9999`, `Story:` and `Story-ID:` trailers.
This is a host-wide default across repositories; portable UUIDs do not sync
manager leases or threads across machines.

## Resolve the manager

- Never choose a default manager or guess an identity.
- If the user gave an exact manager, verify it with `bashy agent list`.
- If the manager is missing or ambiguous, inspect `bashy agent list`, present
  the relevant canonical names, and ask the user to choose before mutating the
  sprint.

## Start a new sprint

Before taking or starting the seat, inspect the affected code, read every linked
story, and write or update the master execution plan. Record both orientation
facts on the card; Bashy refuses `take` and `start` when either is absent:

Before assigning work to a shared test host, droplet, device, or account, take
its host-local advisory hold with `bashy claim NAME --intent TEXT`; release it
when done and never force another holder off without contacting them.

```text
bashy sprint edit ID --primary-goal "ONE OUTCOME" --spec docs/sprint-ID-master-execution-plan.md
```

Then run:

```text
bashy sprint start ID --owner NAME --instruction TEXT
```

Pass the complete user instruction as one argument, preserving its bytes. Do
not interpolate it into shell source. A successful response must name the
sprint, owner, managed session, and Meet contact.

## Direct an active sprint

Inspect it with `bashy sprint show ID`. Preserve its current owner, then run:

```text
bashy sprint instruct ID --instruction TEXT
```

Do not supply or change an owner on this path. The command must reuse the
current owner's managed session.

Sprint stories use one fail-closed path: `sprint claim`, then `sprint submit`
with delivery evidence, then the current manager runs `sprint accept` with the
verification it independently checked. Generic `todo done` never closes a
sprint story.

Stop and report the command error if launch or instruction delivery fails. Do
not claim that work was dispatched unless Bashy confirms it.
