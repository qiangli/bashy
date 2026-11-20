---
id: 309381beda57
kind: bug
title: agentos tests leak 'agentos.test mcp serve --listen loopback:22750' processes (8 found, up to 1 day old)
seq: 3271
status: todo
priority: p2
created: 2026-11-20T23:02:58.677428Z
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

2026-11-20 conductor found 8 orphaned agentos.test 'mcp serve --transport http --listen loopback:22750' processes (ages 18 h to 1 day+) from earlier test runs; killed them. A test starts an MCP HTTP server and does not reap it on failure/timeout paths (same family as 91f04d61). Root cause first; fix in the test cleanup (process group kill + t.Cleanup); regression asserts no listener survives.
