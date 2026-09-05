---
id: fb47ef63021e
kind: task
title: 'Human lane M1b: a person cannot hold an inbox watch, so a human manager cannot keep their seat live'
seq: 219
status: todo
priority: p0
created: 2026-09-05T19:08:17.451401Z
sprint: 126
---

FOUND 2026-09-05 validating the three sprint operating modes. Same M1 axis as the resolver work; the last surface still refusing a person.

REPRODUCED, hermetic scratch HOME, registered person only:
  $ bashy inbox --as human1 --watch
  inbox: watcher identity "human1" is not a registered Bashy agent;
         register it with `bashy agents add` or choose one from `bashy agents list --all`

Two things wrong. The refusal itself, and the advice: it tells a HUMAN to register as an AGENT, which is the same misdirection the one-principal work removed everywhere else (pointing a person at a list they can never appear in).

WHY IT MATTERS RATHER THAN BEING COSMETIC. Holding an inbox watch IS the attached transport rung — "an agent holding its own inbox watch open has undertaken to read it" — and reading mail is also what refreshes the seat (RefreshSprintOwnerActivity). So a human sprint manager cannot keep their own seat live, and `sprint take` recommends the exact command that refuses them:

  next: ... `bashy inbox --as <owner> --watch` (reads your mail and keeps the seat live)

A human can already OWN a sprint (fixed), READ their inbox (fixed, --peek and plain reads work), and AUTHOR on mb (fixed). Only the WATCH refuses, which is the one that maintains liveness.

WHERE: bashy/internal/agentos/inbox.go, inboxWatcherClaim — the reader check next to it was already widened to accept any non-role principal; this claim path was not.

CARE NEEDED, which is why this is filed rather than patched blind. The watcher claim is not just a name check: it files a room card and carries SessionClaim/OwnerPID so a second process cannot read as an identity it does not hold. A person needs a card for the attached rung to mean anything, but the impersonation guard exists to stop one AGENT harness claiming another AGENT name, and a person is not an agent identity. Establish which parts of the claim apply to a person before widening it, and pin the answer with a test.

DO NOT widen it by simply dropping the registration check — that would let any unregistered string hold a watch and file a card.
