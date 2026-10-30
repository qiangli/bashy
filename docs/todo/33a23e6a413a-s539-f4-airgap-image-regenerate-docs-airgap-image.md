---
id: 33a23e6a413a
kind: chore
title: 'S539.F4 airgap-image: regenerate docs/airgap-image.md table (act, curl rows)'
seq: 2851
status: todo
priority: p2
labels:
    - release
created: 2026-10-30T07:43:53.332834Z
sprint: 539
sprint_id: 54bd555e-d487-573f-b2d4-a89ca4d84a8f
sprint_title: 'rc.23 follow-ups: known issues, release hygiene, promotion'
---

Sprint 539 (rc.23 follow-ups), filed 2026-10-30 so Sprint 525 can close. Operator approved known issues 1, 2, 4 and 5 for bashy v1.0.0-rc.23 (ledger in F0). The airgap-image workflow fails only on doc drift. The gate measured fail=0 (343 work offline, 58 offline by design), but docs/airgap-image.md differs for the 'act' and 'curl' rows. It has been failing since rc.22. Regenerate with AIRGAP_WRITE_DOC=1, commit, and confirm the workflow is green. A direct fix, not a known issue.
