---
id: 6995da481f75
kind: bug
title: S513.1c Matrix E2E hermetic preflight and idempotent setup repair
seq: 2744
status: done
priority: p0
created: 2026-10-27T17:58:08.413267Z
weave: 309
assignee: codex-gpt6-luna
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
closed: 2026-10-27T18:51:53.558812Z
closed_by: s513-manager
---

Follow-up to manager request-changes on Bashy weave #298 commit 31d39635. In examples/matrix-chat/provision.py, setup rerun resets every existing Matrix account password, risking active sessions; make normal setup skip existing users and grant admin only on initial creation, fail closed if user enumeration fails. In examples/matrix-chat/e2e.py, preflight calls bashy tool install tuwunel and may download; use a read-only signed installed-tool lookup and fail closed if absent. Preserve bounded synthetic secret adapter and real signed service, no unrelated changes. Add focused tests that assert no password reset on rerun and no install on missing dependency. Run Python unit tests plus actual signed tuwunel two-user text/image/idempotency/restart/media E2E on claimed novidesign after S514 push contention check. Commit with Sprint 513 and story trailers; submit exact test evidence normally. Manager grades and merges both #298 and this repair; no original Matrix product acceptance until combined and Cloudbox HTTPS gates.

The new worker base starts at Bashy main and lacks rejected #298. Fetch the
exact worker commit from
`/Users/qiangli/.bashy/weave/bashy-6497d06f/workspaces/issue-298`
(`31d396355f91efe52815d535171365313cee4685`) into the isolated
workspace, then commit this repair on top. Do not edit #298 workspace or
shared main. Preserve #298 author attribution in the merged history.
