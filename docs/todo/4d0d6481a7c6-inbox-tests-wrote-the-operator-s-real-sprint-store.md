---
id: 4d0d6481a7c6
kind: task
title: inbox tests wrote the operator's REAL sprint store, propping up a ghost conductor
seq: 21
status: done
priority: p0
created: 2026-09-02T20:03:58.15585Z
sprint: 105
closed: 2026-09-02T20:04:01.442752Z
---

isolateUnifiedInbox redirected four stores and not the fifth, and the tests read as a REAL baseline agent (codex-gpt5.6-sol) because meet.Create refuses an unregistered participant. runUnifiedInbox opens with weave.RefreshSprintOwnerActivity(reader), which writes the sprint lease that name holds — so every 'go test ./internal/agentos/' renewed a live conductor lease on the host board for another full TTL. That, not an orphaned daemon, is what kept sprint 98 reporting healthy with nothing running. Fix: isolate BASHY_HOME + BASHY_SPRINT_DIR, and register a test-OWNED agent in the already-isolated fleet ring instead of borrowing a live identity. Guard asserts the outcome (the real store's mtime/size is unchanged) rather than a checklist of stores, and was verified to FAIL when the leak is reintroduced.
