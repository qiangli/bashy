---
id: b76d48d2b5b9
kind: doc
title: S514.R7 generic fast-loop practices in bashy skills/help (conductor, dag, mod/dag sync, sprint)
seq: 2733
status: assigned
priority: p1
labels:
    - skills
created: 2026-10-27T15:59:33.303376Z
weave: 293
assignee: claude-sonnet5.5
sprint: 514
sprint_id: fadd1a46-0368-50f1-a65d-4ed147b17629
sprint_title: 'invitation email + scoped satellites (KISS/MVP: scope + expiry on the one-time pairing code; no outpost change)'
---

Operator 2026-10-27: practices that apply to any project belong in bashy's own guidance, not only in the dhnt runbook (umbrella docs/kb/pages/release-bashy.md 'Fast loop'). Add short, generic, project-agnostic text (no dhnt names, no hostnames) where agents read it:
- skills/bashy/commands/conductor.md (conductor skill): (1) fix briefs for a platform-specific failure must carry native host access and require a native run of the failing test before submit; CI confirms, it is not the debugger. (2) tests must be hermetic (no downloads); re-run a single failed CI lane once automatically, two failures is a bug. (3) disposable candidate tags (-dev) may be tagged as soon as pre-flight is green so release builds overlap CI; promotion still needs every gate green.
- dag help/skill (where 'bashy dag --mesh' is documented): pre-flight a candidate on peers in parallel (one target per OS on the peer that has it) before pushing; prefer the biggest peer for heavy suites; keep the dev box for edits and short unit tests.
- mod sync / dag sync help: commit record/docs changes in a producer before bumping its consumers; drift compares exact HEADs, so a later docs-only producer commit makes every consumer pin stale.
- sprint help (if it lists manager practices): one line pointing to the conductor skill for the above.
Keep each addition to 2-5 lines; follow each file's existing style; regenerate any golden/projection the tests check (adoption projections, skill footers). Gate: go test -short ./internal/agentos (clean PATH) + the skill/help golden tests.
