---
id: a1962b2398da
kind: enhancement
title: Add opt-in fail-closed POSIX harness DAG gate
seq: 335
status: done
priority: p0
labels:
    - posix
    - release
    - dag
created: 2026-09-24T00:25:10.51005Z
assignee: codex-s250
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
closed: 2026-09-24T02:13:00.092029Z
closed_by: codex-s250
---

User directs Bashy dag.md to document and invoke the licensed POSIX shell-only harness through an explicitly configured local path and supervisor approval. Implement an opt-in target using the new pure strict bin/sh artifact. Target must fail closed for missing harness, approval, preflight, runner, or red 493 TP verdict; must not expose licensed source/material in public repo or change original TP limits/providers. Document exact steps, provenance, and VSC run interface. Keep normal GNU Bash86 and default DAG targets unaffected. Coordinate private command contract with Confirm, then focused fail-closed controls and full approved VSC493 acceptance before release.

Acceptance: public DAG target at Bashy 7416f0b rejects absent harness and propagated wrapper failure in focused controls. Approved private harness 8b0, umbrella approval 9d886fc, and exact Bashy 7416f0b/sh 72bb8cd source completed `bashy dag test-posix-shell` exit 0 (`dag: 1 target(s) ok`), sealed 493/493 PASS group, zero missing/caps/runner failures/blockers, and 24/24 retrieval checksums; ledger SHA-256 6ac2daf9ec384566e295e78182780885f9f964ecd87794183fa6e2fb0fe37aad. Only the shell set ran; utility sets are next-release Sprint 266 Story #746.
