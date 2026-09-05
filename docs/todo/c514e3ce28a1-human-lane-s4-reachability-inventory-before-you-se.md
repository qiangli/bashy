---
id: c514e3ce28a1
kind: task
title: 'Human lane S4: reachability inventory before you send'
seq: 212
status: todo
priority: p1
created: 2026-09-05T15:47:51.84164Z
sprint: 126
---

The grounding stage: survey who can actually receive before choosing whom to message. Add 'bashy sprint who --human' listing every sprint owner with live/stale/unreachable, last inbox-ack, and unread depth. Gate: the board's aggregate '[UNREACHABLE: 2]' resolves to a per-owner reason for #86/#100/#101/#122; output is deterministic (no model call) and derived from existing room/agent state.

Implementation home: `coreutils/pkg/weave`, reading existing dhnt room, agent,
and acknowledgement state. It must not probe or depend on an external project or
service.
