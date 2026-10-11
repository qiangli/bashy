---
id: 14952455911e
kind: feature
title: S11 Codync typed HTTP declaration and fake host verification
seq: 1891
status: todo
priority: p1
labels:
    - bashy
created: 2026-10-11T08:46:56.642929Z
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
---

S11 scoped continuation of ee69f18dbb08. Previous weave #29 ended no-op because the first typed HTTP transport could only bind whole {args}; its blocked evidence is retained in the sprint thread. Yoke generic typed argument binding, enum projection, and shared --yes mutation admission are independently verified on Mac/Linux, accepted at 7028891, and pinned into Bashy 8c9931a9 through Yoke ef6a860. Implement examples/fleet/tools/codync.yaml through mode: http and native separate request fields, never shell/eval/curl wrappers or tool-name branches. Preserve official Codync v2.13.2 Apache-2.0 release digest and exact bearer token-file handling; bots/send/approve/usage must invoke POST /api/METHOD as appropriate, declare MCP team and running-host availability. Only edit this YAML, adjacent documentation, and focused tests. Worker verify: isolated registry import/show/list --json, fake loopback host enforcing token and exact effect/approval behavior; go test ./internal/fleet/... if relevant. Do not stamp verified; manager owns live Codync host, reverse ACP, and remote gates. All commits with Sprint #418, Story-ID and Agent trailer. Dragon short tests only.
