---
id: 7fbc200115f2
kind: enhancement
title: S539.F5 agentos tests exceed the 10m preflight budget on noviwin1 (known issue 4)
seq: 2852
status: todo
priority: p2
labels:
    - release
created: 2026-10-30T07:43:53.490912Z
sprint: 539
sprint_id: 54bd555e-d487-573f-b2d4-a89ca4d84a8f
sprint_title: 'rc.23 follow-ups: known issues, release hygiene, promotion'
---

Sprint 539 (rc.23 follow-ups), filed 2026-10-30 so Sprint 525 can close. Operator approved known issues 1, 2, 4 and 5 for bashy v1.0.0-rc.23 (ledger in F0). Known issue 4. The release preflight on noviwin1 ran 1181 PASS / 41 SKIP with 0 failures, but the agentos package hit its 10m budget, so the script exited 1. Fix by splitting the package's tests or raising the per-package budget for the Windows lane, aligned with Sprint 500's layered gates (S500.2). Then close the ledger entry.
