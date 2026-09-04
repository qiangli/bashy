---
name: sprint
description: Start or direct a Bashy sprint while requiring an explicit canonical project manager and managed delivery. Use when the user invokes /sprint or asks an agent to create, start, or direct a sprint.
---

# Sprint

Translate the user's request into Bashy sprint commands. This skill is the
agent-facing `/sprint` adapter; do not build or invoke a second prompt parser.

## Resolve the manager

- Never choose a default manager or guess an identity.
- If the user gave an exact manager, verify it with `bashy agents list`.
- If the manager is missing or ambiguous, inspect `bashy agents list`, present
  the relevant canonical names, and ask the user to choose before mutating the
  sprint.

## Start a new sprint

Create or plan the sprint with the existing `bashy sprint` commands, then run:

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

Stop and report the command error if launch or instruction delivery fails. Do
not claim that work was dispatched unless Bashy confirms it.
