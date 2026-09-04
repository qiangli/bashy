---
id: ce68ace0a6fd
kind: task
title: Sprint 114 Bash# final integration
seq: 199
status: done
priority: p0
created: 2026-09-04T00:40:40.649684Z
weave: 15
assignee: qiangli
sprint: 114
closed: 2026-09-04T01:56:20.460461Z
---

Update .sibling-pins to the final pushed sh Sprint 114 HEAD. Fix the bash front door so `bash --posix --bashpp` is accepted but Bash++ is inert and byte-identical to plain `bash --posix`, matching bashpp-tests Sprint 114 acceptance; update the contradictory internal refusal test. Preserve `bashy check --bashpp` null-safety behavior and all Classic/POSIX behavior. Build against the pinned sh and run focused/full tests plus the Bash# acceptance harness when final sh is available. Do not touch unrelated agentos, docs, conductor, or sprint files. Commit with exact trailers: Sprint: #114; Story: #173; Story-ID: c6a1540bd339.
