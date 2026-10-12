---
name: sprint
description: Start or direct a Bashy sprint while requiring an explicit canonical project manager and managed delivery. Use when the user invokes /sprint or asks an agent to create, start, or direct a sprint.
---

# Sprint

Translate the user's request into Bashy sprint commands. This skill is the
agent-facing `/sprint` adapter; do not build or invoke a second prompt parser.

## Select a sprint

Every sprint follows the same lifecycle, authorization and acceptance rules.
No maintenance number or fixed maintenance UUID is reserved. Use a sprint
number for a specific card or `last` for the most recently created card.
Commands also accept `--sprint N` or `--sprint last` instead of the sprint
operand. Do not combine the flag and operand. Omitting selection uses the
same last card; it never creates one implicitly.
An empty board requires creating a sprint before selecting it.

Repository todos use the selected sprint: `todo add "fix something" --sprint N`
or `--sprint last`. Use `--no-sprint` for an unlinked todo. Personal todos remain
unlinked by default. `todo edit ID --sprint 0` unlinks an existing item.

Read the card and story, coordinate with its manager, and move backlog/done to
doing before claiming. Filing a story does not reopen a sprint or appoint a
manager. Normal story scope, evidence and commit trailers still apply.

```text
bashy sprint show last
bashy todo add "fix something" --sprint last --note "Problem, behavior, verification"
bashy sprint claim last STORY_ID --owner YOUR_REGISTERED_AGENT
bashy sprint submit last STORY_ID --as YOUR_REGISTERED_AGENT -m "delivery evidence"
bashy sprint accept last STORY_ID -m "manager verification"
```

## Resolve the manager

- Never choose a default manager or guess an identity.
- If the user gave an exact manager, verify it with `bashy agent list`.
- If the manager is missing or ambiguous, inspect `bashy agent list`, present
  the relevant canonical names, and ask the user to choose before mutating the
  sprint.

## Start a new sprint

Create the card with `bashy sprint add "Title"` (or add `--json` for structured
output). Bashy allocates and returns the next available number; do not supply
a number or a `next` selector. Use that returned ID for the plan and lifecycle
commands below. Sprint 9999 is historical and has no special maintenance role.

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
