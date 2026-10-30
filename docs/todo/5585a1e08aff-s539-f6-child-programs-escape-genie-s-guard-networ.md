---
id: 5585a1e08aff
kind: bug
title: S539.F6 Child programs escape genie's @guard network deny (known issue 5; umbrella todo db50b5e6)
seq: 2853
status: todo
priority: p1
labels:
    - release
created: 2026-10-30T07:43:53.647156Z
sprint: 539
sprint_id: 54bd555e-d487-573f-b2d4-a89ca4d84a8f
sprint_title: 'rc.23 follow-ups: known issues, release hygiene, promotion'
---

Sprint 539 (rc.23 follow-ups), filed 2026-10-30 so Sprint 525 can close. Operator approved known issues 1, 2, 4 and 5 for bashy v1.0.0-rc.23 (ledger in F0). Known issue 5, a documented security limit. Full scope is in umbrella todo db50b5e6. bashy children inherit the guard (like the A5d boundary via BASHY_APP_BOUNDARY, fail closed); non-bashy programs need a declared, granted @runs; explicit @sandbox for real containment. Red/green on native Windows and Linux.
