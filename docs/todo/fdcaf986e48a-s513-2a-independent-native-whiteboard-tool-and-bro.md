---
id: fdcaf986e48a
kind: feature
title: S513.2a independent native whiteboard tool and browser E2E
seq: 2735
status: done
priority: p0
created: 2026-10-27T17:00:03.71266Z
weave: 296
assignee: muse-conductor
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
closed: 2026-10-27T18:52:10.533157Z
closed_by: s513-manager
---

Implement code portion of parent9b12aa095f57. WHITEBOARD is an independent general-purpose tool, NOT a tab/component of brainstorm. Add examples/whiteboard with canonical whiteboard.bsh command/skill duality, registration, native Go server/browser, its own default port, URL and data directory, serve/status/backup/stop or clear lifecycle instructions. Start/stop independently with brainstorm/Matrix absent; all three can run concurrently. Generic facilitator/participant roles; persistent named sessions; draw freehand + notes; realtime multi-browser updates/reconnect; facilitator clear/lock; PNG/JSON exports; account-free invited participant capability and private facilitator capability; isolated sessions and server-enforced permissions. Reuse patterns from examples/brainstorm but do not edit its files in this story. No CDN, containers, core command or new service dependency. Use relative/base-aware URLs for outpost app prefix. Shared code only if it does not create runtime dependency; keep minimal. MUST ADD automated real-browser happy-path E2E using existing Playwright style: create board, at least three contexts draw/add notes and see sync, lock/clear permission, restart same data, export valid PNG/JSON; prove standalone and paths under a prefix. Add focused native tests for persistence, concurrency, bounds/access and write failure. Do not touch original classroom data. Native peer test access supplied privately by manager; no containers/heavy builds on dev host or paid infrastructure. Read local CLAUDE.md; implement/commit in isolated repo with injected identity and supplied trailers, send tests/commit/blockers. Budget one bounded initial run. Follow-on generic brainstorm changes are a separate sequential task.
