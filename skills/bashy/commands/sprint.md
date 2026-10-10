---
name: sprint
description: Start or direct a Bashy sprint while requiring an explicit canonical project manager and managed delivery. Use when the user invokes /sprint or asks an agent to create, start, or direct a sprint.
---

# Sprint

Translate the user's request into Bashy sprint commands. This skill is the
agent-facing `/sprint` adapter; do not build or invoke a second prompt parser.

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
