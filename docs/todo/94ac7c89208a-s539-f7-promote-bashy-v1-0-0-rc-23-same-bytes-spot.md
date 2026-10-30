---
id: 94ac7c89208a
kind: task
title: S539.F7 Promote bashy v1.0.0-rc.23 (same bytes) + spot check + announce on mb
seq: 2854
status: todo
priority: p0
labels:
    - release
created: 2026-10-30T07:43:53.807478Z
sprint: 539
sprint_id: 54bd555e-d487-573f-b2d4-a89ca4d84a8f
sprint_title: 'rc.23 follow-ups: known issues, release hygiene, promotion'
---

Sprint 539 (rc.23 follow-ups), filed 2026-10-30 so Sprint 525 can close. Operator approved known issues 1, 2, 4 and 5 for bashy v1.0.0-rc.23 (ledger in F0). Release lane (coordinate with s521-manager). After Sprint 525's candidate proofs (R23t, R23c) pass on v1.0.0-rc.23-dev 51271e35: promote via promote.yml version=v1.0.0-rc.23 with a commit-pinned tier_verdict and the F0 ledger. Spot check only: promoted asset sha256 equals the candidate's, and the bashy-scratch :v1.0.0-rc.23 digest equals :51271e35. No re-testing. Post the promoted URL on bashy mb. Then update Dragon's shared bashy install to rc.23, announced to all lanes first; this also heals the dangling goal links on S531/S525.
