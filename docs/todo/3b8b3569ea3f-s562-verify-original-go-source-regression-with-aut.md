---
id: 3b8b3569ea3f
kind: test
title: S562 verify original Go-source regression with authenticated single-binary candidates
seq: 3076
status: assigned
priority: p0
created: 2026-11-05T02:36:17.536854Z
assignee: s562-go-regression
sprint: 578
sprint_id: 70ba48be-af89-5ab1-8a8d-42a15fc1ddbe
sprint_title: 'S562 follow-up: remaining fence corpus work'
---

Operator requires no regression of previously green original Go-source Tour 97 and GbE 85, each native/interpreted/compiled. Current product Makefile builds a single cmd/bashy binary but original harnesses require Unix launcher plus .real (GbE rejects equal hashes). Implement narrow explicit candidate layout support for current native single-binary Bashy, preserving legacy pair validation, exact SDK/source identities, clean checkout checks, reviewed candidates.tsv admission and offline evidence validation. No wrapper shim for acceptance, reduced denominators, timeout growth or relaxed hashes. Add focused positive and tamper/refusal tests. Then manager runs both original gates on claimed test host with exact candidate and retained evidence. Fix only harness compatibility here; any measured product regressions receive owning-layer repairs and original cases rerun. Public docs use roles and repository-relative paths only.

Carried from S562 (operator-approved early close 2026-11-06); deferral is not PASS. Run/branch/evidence references: see existing story details and S562 board; no verified run or branch reference recorded.
