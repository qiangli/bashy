---
id: a1962b2398da
kind: enhancement
title: Add opt-in fail-closed POSIX harness DAG gate
seq: 335
status: todo
priority: p0
labels:
    - posix
    - release
    - dag
created: 2026-09-24T00:25:10.51005Z
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

User directs Bashy dag.md to document and invoke the licensed POSIX shell-only harness through an explicitly configured local path and supervisor approval. Implement an opt-in target using the new pure strict bin/sh artifact. Target must fail closed for missing harness, approval, preflight, runner, or red 493 TP verdict; must not expose licensed source/material in public repo or change original TP limits/providers. Document exact steps, provenance, and VSC run interface. Keep normal GNU Bash86 and default DAG targets unaffected. Coordinate private command contract with Confirm, then focused fail-closed controls and full approved VSC493 acceptance before release.
