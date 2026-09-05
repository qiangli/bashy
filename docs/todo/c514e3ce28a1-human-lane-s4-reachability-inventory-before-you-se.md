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

VALIDATION 2026-09-05 — MECHANISM RETARGETED, gate unchanged. The original card
proposed `bashy sprint who --human`. Measured: `sprint who` reports per-RUN,
per-repo rows (run/sprint/agent/title) and carries no owner or reachability
column, so `--human` would be a NEW report behind a new flag — feature creep for
data the host already has.

Do it without a new surface: the sprint board already prints the aggregate
`[UNREACHABLE: N]`, and `bashy whois <owner>` already returns a ranked contact
ladder with liveness and confidence. Resolve the EXISTING aggregate to its
per-owner reasons in `sprint show`/board rendering, reusing whois + room state.
No new verb, no new flag, no new store; the gate below is unchanged.
