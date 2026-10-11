---
id: d1438073d581
kind: feature
title: S4 bridge Bash# @confirm yield into the shared persisted approval queue
seq: 1890
status: todo
priority: p1
labels:
    - bashy
    - approval
created: 2026-10-11T07:19:35.302883Z
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
---

Scoped child of original S4 Story-ID 68e77d5cfa95. The yoke S4 branch provides pkg/ask persisted approval queue and ACP/chat consumers, but Bash# internal/agentos/confirm.go still yields without persisting a request. On Bash# @confirm yield, write the same token/argv/effects/requester/sprint/session approval record through the shared ask contract. Resume --confirm=TOKEN:yes|no must consume the same decision once; default auto remains unchanged. No separate log or old format compatibility. Own only Bashy confirm/interpreter code; coordinate with S16 supervisord worker. Verify focused go test ./internal/agentos/... on claimed native host and process-level @confirm yield -> bashy approve list/allow -> same-token resume, plus deny and duplicate-token tests. Commit with Sprint #418, this Story-ID, Agent trailer; manager will run native gate.

Manager contract correction: the yoke S4 provider commit `76119a7` makes `ask.Resolve(source, token, session, argv)` require exact session and argv because the existing @confirm token is only 32 bits. Pass the same stable session and exact operation argv to `ask.Raise` and `ask.Resolve`; a decision from a different session or argv must never authorize this operation. The original S4 remains open until this Bash# bridge and its live gate pass.
