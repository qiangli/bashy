---
id: 2c49fae01ec9
kind: bug
title: 'S539.F2 noviwin1: TestTmplFenceEngineNonCompliantIsRefused ''[: command not found'' (known issue 1)'
seq: 2849
status: todo
priority: p2
labels:
    - release
created: 2026-10-30T07:43:53.017474Z
sprint: 539
sprint_id: 54bd555e-d487-573f-b2d4-a89ca4d84a8f
sprint_title: 'rc.23 follow-ups: known issues, release hygiene, promotion'
---

Sprint 539 (rc.23 follow-ups), filed 2026-10-30 so Sprint 525 can close. Operator approved known issues 1, 2, 4 and 5 for bashy v1.0.0-rc.23 (ledger in F0). Known issue 1. On noviwin1 only, the test fails with '[: command not found' from shPath sh. rc.22's test binary fails identically there, and bashy CI windows-latest passes it. Root-cause the host or fixture difference (which sh resolves; PATH; MSYS vs bashy sh), fix it so the test is host-independent, then close the ledger entry.
