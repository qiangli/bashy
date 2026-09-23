---
id: f00085a7efe8
kind: bug
title: Investigate Windows Go Tour default-selection interpreted count
seq: 326
status: assigned
priority: p1
labels:
    - windows
    - tour
created: 2026-09-23T12:17:51.170599Z
assignee: codex-gpt5.6-sol
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

Windows Go Tour full executor on bashsharp-tests 90885b9, candidate Bashy 5353ba3/sh 3d5559e: _content/tour/concurrency/default-selection.go interpreted yielded default_count=9 against native oracle range 10..10. Baseline 97/97 and compiled 97/97 pass. Evidence ledger SHA-256 aabb393b19c2a25a2cf54fa502d20489284698693192e2dfc53ccf2e3a09affb, root 5d22f443f9505a1b28845ee5baf22e6e978a929dc2420ceff6fff56e93d8c4ec. Reproduce exact row repeatedly under unchanged fixture and semantic comparator to distinguish scheduler nondeterminism from runtime defect; repair root cause if repeatable, then rerun affected Windows Tour gate with final product candidate. Preserve existing deadlines and source.
