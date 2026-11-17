---
id: 4b22a277ee4e
kind: refactor
title: S0G Port required release-preservation gates to make-free DAG targets
seq: 3240
status: done
priority: p0
labels:
    - layer-separation
created: 2026-11-17T22:01:17.140744Z
weave: 108
assignee: agy-gemini3.8-flash
sprint: 575
sprint_id: 65ce85f5-87c0-5863-bf55-62de86f5df36
sprint_title: 'Layer separation: Classic <- Bash# <- Yoke import-graph delineation'
closed: 2026-11-17T23:38:45.432933Z
closed_by: s575-codex-manager
---

Sprint 575 tooling-only independent implementation slice, before engine changes. Operator approved minimum necessary changes and standalone Bash <- Bash# <- bashy. Preserve all product behavior and test assertions. Scope ONLY bashy/dag.md and minimum make-backed helper call sites needed for former test parity: scripts/test-build-fail-closed.sh, test-bash-container-mode.sh, verify-bashy-scratch.sh, build-conformance-image.sh and fixture/build wrappers if strictly needed. Implement missing DAG prerequisites matching the existing Makefile test contract, test-bash-fixtures, test-bash-container modes and aggregate test-bash-gate, scratch build/verify and existing build/install variable wiring where the script checks require it. Make-free means no DAG target or migrated helper falls through to make. Keep the Makefile for its separately owned global removal story; no broad docs sweep or dispatch/make fallback changes. Keep failure injection/stale-artifact/mode assertions unchanged; update only runner invocation and fixture wiring necessary to exercise DAG. No Go engine/CLI product code, dependency bumps, image locks, source-module path migration or unrelated fixes. Do not run builds/full tests/foreign suites on dev box; static bashy dag --check is allowed. Official-image gate runs are scheduled by manager on gate host; record exact commands needed and honestly mark not-run. Read bashy CLAUDE.md and umbrella current plan. Work in isolated weave workspace and commit only this slice with actual injected agent identity and Sprint/Story/Story-ID trailers; manager reviews/gates/merges/pushes. No remote publication yourself. Prior gate audit be8c35fd5732 found eight additional script checks and missing 2-mode DAG gate; current release ledger records three make-backed checks omitted by operator in rc.37. Restore their coverage without weakening assertions.
