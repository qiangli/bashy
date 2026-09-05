---
id: f0ebc6c9a11a
kind: task
title: 'Human lane S6: delivery state is an audit column, never a repair loop'
seq: 214
status: todo
priority: p1
created: 2026-09-05T15:47:51.886112Z
sprint: 126
---

Every human-authored message (mb post/send, meet tell, dm) carries sent -> delivered -> acked, visible to the sender and in inbox. Gate: an unacked message stays visibly unacked indefinitely; a test asserts NO code path re-sends, re-words, auto-escalates or re-routes it. Rationale on the record: delivery state is reported to the SENDER and to no automation - an auto-retry would make a dead owner look reachable, which is the precise condition this sprint exists to expose.

Implementation home: the existing `coreutils/pkg/bus` receipt model, meet
adapters in `coreutils/pkg/meet`, and bashy inbox/front-door presentation. The
final gate must exercise both CLI and the existing `bashy apps serve` browser
path and observe the same state transitions; it must not introduce a second web
receipt model.
