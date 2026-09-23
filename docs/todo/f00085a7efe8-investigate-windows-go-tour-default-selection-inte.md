---
id: f00085a7efe8
kind: bug
title: Investigate Windows Go Tour default-selection interpreted count
seq: 326
status: done
priority: p1
labels:
    - windows
    - tour
created: 2026-09-23T12:17:51.170599Z
weave: 19
assignee: codex-s250
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
closed: 2026-09-23T18:18:32.74217Z
closed_by: codex-s250
---

Windows Go Tour full executor on bashsharp-tests 90885b9, candidate Bashy 5353ba3/sh 3d5559e: _content/tour/concurrency/default-selection.go interpreted yielded default_count=9 against native oracle range 10..10. Baseline 97/97 and compiled 97/97 pass. Evidence ledger SHA-256 aabb393b19c2a25a2cf54fa502d20489284698693192e2dfc53ccf2e3a09affb, root 5d22f443f9505a1b28845ee5baf22e6e978a929dc2420ceff6fff56e93d8c4ec. Reproduce exact row repeatedly under unchanged fixture and semantic comparator to distinguish scheduler nondeterminism from runtime defect; repair root cause if repeatable, then rerun affected Windows Tour gate with final product candidate. Preserve existing deadlines and source.

Follow-up: weave #19 repeated the interpreted count of 9 on the original candidate. The related sh Story #142 main-task sleep-bridge patch produced one later Windows count of 9, but a per-row binary hash and matched repeated old/patched controls were not captured. Treat that patch as an unproven experiment, leave #326 and #142 open, and require an authenticated exact-row repeat before integration. No Tour fixture, comparator, or deadline changed.

Resolution, 2026-09-23: sh `69eda1b3d7c53ae15c004e529d20faeeea93caa3` and Bashy `f93816e28551a7f61193a3ee47312bccf653af67` resolve the interpreted default-selection timing. The clean baseline yielded nine defaults in 5/5 matched runs; the pure-duration candidate yielded ten in 35/35, and the integrated current-head candidate yielded ten in 10/10. An authenticated focused row passed all three modes against seven native oracle runs. Final public-head Windows Go Tour on harness `9379584eeaa20a230259fec78c7b53b6f7d22519` passed **291/291** (97 per mode); independent validator PASS. The original source, comparator, and 60-second step limit were unchanged. Candidate binary SHA-256 `36960b3320a29aa2f4727fd80234b3d7cba964960ec4047cf3202256d4a969b0`; manifest SHA-256 `592892ffdca85e12091e401ffb46a197e34594afa922c7de51f44fa60e9b3f31`; evidence root `f5028238568575293266cb2c18b8eeb3f78bd58e0412e91f6d8f2e21cf5106d6`; ledger SHA-256 `0389185d1346073c1c1ed1c8fbf0e754b8071a7fb4b7a1bfd4401290b049e0d4`; run-log SHA-256 `de39d607e3f904a965501412e56ecca5ed346645b8a71e9fd31d52c3459c7e8e`; validator-log SHA-256 `ac343ced5e9b716837e021339eba62aa843f65ab1cd96eb1e631eff01ef5ae89`.
