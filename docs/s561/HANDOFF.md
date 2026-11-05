# Sprint 561 handoff — closed engineering, S571 follow-up

S561 is **done**. Its conductor lease is released. Both disposable licensed
hosts were retired, the source freeze was lifted, and other sprints may proceed.
This replaces the urgent pre-run pickup instructions; do not restart that run
or repeat its completed Windows/public-gate work from the older checklist.

Frozen licensed candidate: bashy `f1bcd711`, sh `74586fde`, coreutils
`211789fa`; harness `0b2b185d`. Profile B: 493/493 PASS group, zero blockers.
Profile D: 117 sets, 9,337 test purposes, 9,270 PASS group, 4 FAIL,
54 UNRESOLVED, 9 INSPECT: **67 non-passes**, no regression.

The [licensed aggregate record](a4-licensed-results.md),
[public-lane report](a4a-results.md), [Windows repair report](a3d-results.md)
and [candidate manifest](candidate-manifest.json) retain the evidence.
Closure commits: bashy `102a3c54`, umbrella `b203e57`. The closure coreutils
pin `601546e1` adds documentation only relative to the frozen source tree.
Both certification build closures checked SAME; exact certification artifact
hashes remain a separate requirement under the [identity rule](evidence-identity.md).

[Sprint 571](https://github.com/dhnt/umbrella/blob/main/docs/plan-s571-posix-certification-readiness.md) owns the 53 symlink setup results, `man:4`, four `more`
failures and nine current-journal manual inspections. S355 authority packets
remain prepared but unsent; S377 decisions need applicable evidence on this
journal. Engineering closure is not full certification readiness or approval.

Runs 17/18 were retired as superseded after their delivery was integrated.
Tips were preserved; local `archive/s561-run-18` retains an unrelated generated
KB change. Other sprints' work was left intact. No fresh licensed run was
performed during closure and no raw result was rewritten.
