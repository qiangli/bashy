---
id: 87c22f17ad9f
kind: bug
title: S539.F3 Make TestTmplFenceEngineNonCompliantIsRefused hermetic (no Go toolchain fetch) (known issue 2)
seq: 2850
status: todo
priority: p2
labels:
    - release
created: 2026-10-30T07:43:53.174408Z
sprint: 539
sprint_id: 54bd555e-d487-573f-b2d4-a89ca4d84a8f
sprint_title: 'rc.23 follow-ups: known issues, release hygiene, promotion'
---

Sprint 539 (rc.23 follow-ups), filed 2026-10-30 so Sprint 525 can close. Operator approved known issues 1, 2, 4 and 5 for bashy v1.0.0-rc.23 (ledger in F0). Known issue 2. On Ubuntu CI the test failed with 'bash++: selected Go toolchain: resolve Go toolchain: gotoolchain: fetch ...', a network-dependent path. Make the test hermetic (pre-provisioned or faked toolchain; no download) per the release fast-loop rule, then close the ledger entry.
