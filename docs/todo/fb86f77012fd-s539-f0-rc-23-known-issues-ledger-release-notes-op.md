---
id: fb86f77012fd
kind: doc
title: S539.F0 rc.23 known-issues ledger + release notes (operator-approved skips)
seq: 2847
status: todo
priority: p1
labels:
    - release
created: 2026-10-30T07:43:52.692698Z
sprint: 539
sprint_id: 54bd555e-d487-573f-b2d4-a89ca4d84a8f
sprint_title: 'rc.23 follow-ups: known issues, release hygiene, promotion'
---

Sprint 539 (rc.23 follow-ups), filed 2026-10-30 so Sprint 525 can close. Operator approved known issues 1, 2, 4 and 5 for bashy v1.0.0-rc.23 (ledger in F0). Record one known-issues ledger for v1.0.0-rc.23 (51271e35). Entries, each with the affected test(s), OS, impact, owner and review date (rc.24): (1) TestTmplFenceEngineNonCompliantIsRefused, noviwin1 only ('[: command not found' from shPath sh; rc.22 identical; CI Windows passes); (2) the same test on Ubuntu CI: 'resolve Go toolchain: fetch' network flake; (4) the agentos package exceeds the 10m preflight budget on noviwin1 (0 test failures); (5) child programs escape genie's @guard network deny (umbrella todo db50b5e6): a documented security limit, no test skipped. Gates skip/xfail exactly these entries; failures outside them still block; unexpected passes are reported. List open known issues in the rc.23 release notes. Rule: skips need operator approval.
