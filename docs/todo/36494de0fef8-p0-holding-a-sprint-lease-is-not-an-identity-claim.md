---
id: 36494de0fef8
kind: task
title: 'P0: holding a sprint lease is not an identity claim — the manager cannot notify anyone'
seq: 227
status: todo
priority: p0
created: 2026-09-06T01:03:06.795888Z
sprint: 127
---

MEASURED: while HOLDING sprint 127 as claude-opus5-webconsole, `bashy notify
claude-opus5-webconsole "x"` fails exit=1 with "authored communication: unattributed agent
session: running under claude with no claimed agent identity".

WHY THIS IS P0 AND NOT COSMETIC: notification is the DELEGATION path. A manager assigns a
story and the assignee is told — that is how work starts. Every `todo --owner` assignment
this session printed "<name> not notified", so every delegation this sprint made was
silent. A manager that cannot tell an agent it has work is not managing, it is
bookkeeping.

THE QUESTION TO SETTLE FIRST, before any code: is holding a sprint lease an identity
claim? Arguments both ways, and the answer decides the fix:
  FOR — the lease is durable, it names one registered agent, `sprint start` already
  REFUSES unregistered and placeholder names, and refreshing it is what proves liveness.
  Nothing else about the seat is provisional.
  AGAINST — a lease says who is ACCOUNTABLE, not which process is running. Letting any
  process that can write the sprint file author as the holder would let a bystander speak
  as the manager, which is the impersonation this sprint exists to prevent.

LIKELY RESOLUTION (verify, do not assume): the lease should satisfy bus.ResolveAuthoredActor
for the HOLDER NAME ONLY, and only while the lease is live and unstale — i.e. the same
predicate `sprint checkpoint` already uses to decide the seat is held. That keeps the
impersonation guard (you may author as the seat you hold, and no other) while unblocking
delegation.

SCOPE: coreutils/pkg/bus/actor.go + whatever the sprint side must expose. Do NOT invent a
second identity system — pkg/principal and the lease both already exist.

GATE: a red test that assigns a story while holding the lease and asserts the assignee
receives it (via bus.UnreadNotifications, never the CLI echo).
