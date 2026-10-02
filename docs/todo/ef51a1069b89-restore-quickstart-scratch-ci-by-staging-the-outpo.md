---
id: ef51a1069b89
kind: bug
title: Restore quickstart scratch CI by staging the Outpost sibling
seq: 383
status: done
priority: p0
created: 2026-10-01T11:39:38.283157Z
assignee: codex-gpt6.1-sol
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
closed: 2026-10-02T09:48:34.357515Z
closed_by: codex-gpt6.1-sol
---

Exact Bashy candidate 99429e7 quickstart-scratch run 36856570706 fails because ../outpost/go.mod is absent in Docker build context. Stage pinned Outpost checkout in quickstart smoke and COPY into builder; validate on CI before Profile D freeze.
