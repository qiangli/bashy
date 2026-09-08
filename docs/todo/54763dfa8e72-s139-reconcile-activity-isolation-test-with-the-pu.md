---
id: 54763dfa8e72
kind: task
title: 'S139: reconcile activity isolation test with the published room home contract'
seq: 250
status: done
priority: p1
created: 2026-09-08T07:59:02.926806Z
assignee: sprint139-manager
sprint: 139
closed: 2026-09-08T09:41:25.723238Z
---

Full delivery make test exposed TestTransportMustBeStubbedNotRedirected asserting room.Dir does NOT honor BASHY_HOME. Existing coreutils82f425b4 deliberately fixed that leak. Update stale harness contract/comment while preserving transport stubbing and direct containment guarantees; no activity or room production change. Gate: focused activity tests and full Bashy make test with current coreutils pin.

Delivery accepted 2026-09-08 at Bashy fe53058 and coreutils 6942196e: owner full make test and focused activity coverage passed; a clean standalone GitHub clone passed build/vet/test plus e2e dispatch; installed clean binary passed the two-seat delivery smoke. The correction preserves all three transport stubs and asserts no room-store side effects. Gate evidence: /tmp/sprint139-delivery/evidence/bashy-publication-final and /tmp/sprint139-delivery/standalone-evidence; installed evidence: /tmp/s139-installed-candidate-v2.
