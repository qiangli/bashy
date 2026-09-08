---
id: 54763dfa8e72
kind: task
title: 'S139: reconcile activity isolation test with the published room home contract'
seq: 250
status: assigned
priority: p1
created: 2026-09-08T07:59:02.926806Z
assignee: sprint139-manager
sprint: 139
---

Full delivery make test exposed TestTransportMustBeStubbedNotRedirected asserting room.Dir does NOT honor BASHY_HOME. Existing coreutils82f425b4 deliberately fixed that leak. Update stale harness contract/comment while preserving transport stubbing and direct containment guarantees; no activity or room production change. Gate: focused activity tests and full Bashy make test with current coreutils pin.
