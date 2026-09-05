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
