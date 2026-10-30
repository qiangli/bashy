---
id: 73f5b066f684
kind: bug
title: 'S539.F1 bashy native git: support ''git push -q'' (audit common short flags)'
seq: 2848
status: todo
priority: p1
labels:
    - release
created: 2026-10-30T07:43:52.850119Z
sprint: 539
sprint_id: 54bd555e-d487-573f-b2d4-a89ca4d84a8f
sprint_title: 'rc.23 follow-ups: known issues, release hygiene, promotion'
---

Sprint 539 (rc.23 follow-ups), filed 2026-10-30 so Sprint 525 can close. Operator approved known issues 1, 2, 4 and 5 for bashy v1.0.0-rc.23 (ledger in F0). Found in the rc.23 release QA (bashy dag release-qa ref-push step): bashy's native git rejects 'git push -q' (unknown shorthand). Same class as S531's git init -q / git -c fixes. Fix push -q and audit the common short flags agents and dags use (push/fetch/clone/commit/log/show/archive: -q, -v, -u, -f, -n, --quiet). The preflight todo 9379259d also notes native git refusing 'git show'/'git archive'; fix those here too. Red/green tests on Windows and Linux; zero setup (no host git).
