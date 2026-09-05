---
id: 0edb34695070
kind: task
title: 'Human lane S3: human-to-manager chat returns a delivery verdict'
seq: 211
status: todo
priority: p0
created: 2026-09-05T15:47:51.819315Z
sprint: 126
---

A human messages a sprint owner in the room or by private DM and learns immediately whether it landed. Gate: to a LIVE owner -> delivered, and the record is visible in that owner's inbox; to a STALE/UNREACHABLE owner -> NOT DELIVERED naming the reason (no live watcher / no inbox-ack since T), the message still written to the durable board, and NO silent queue. Reproduce against the current board, which carries UNREACHABLE on #86/#100/#101/#122 and 1 live agent vs 3 stale.

Implementation home: reuse `coreutils/pkg/bus` delivery states from
`coreutils/pkg/meet`, with only the necessary callback/front-door wiring in
`bashy/internal/agentos`. The referenced sprint numbers are dhnt host-state test
fixtures, not implementation dependencies.

VALIDATION 2026-09-05 — VOCABULARY CORRECTION, binding. This story sits inside
the Yoke entry-gate OPEN CLASS (bashy-yoke-framework.md §Entry-gate status:
behavior fixes inside today's packages that make an existing Yoke surface tell
the truth about identity, ownership or delivery). That section states the
`Delivery` vocabulary is ALREADY BINDING on the open class: "a surface may not
invent a second delivery ladder now that the kernel would have to reconcile
later."

The canonical states are six and only six (§Communication contract, and adopted
again by dhnt/docs/mb-addressing-model.md): accepted, queued, delivered, read,
failed, unverified.

So "NOT DELIVERED" is NOT a state and must not become one. It collapses `failed`
(proven not to have landed) with `unverified` (no cursor at all — nothing can
prove either way), and those are the two the ladder most deliberately separates.
Reporting an unprovable send as NOT DELIVERED asserts a fact the host cannot
observe, which is the evidence invariant inverted just as surely as reporting it
delivered would be. Gate amended: a send to a live owner reports `delivered`; to
a stale/absent owner it reports `failed` or `unverified` WITH the reason, and the
two are distinguishable in the output.
