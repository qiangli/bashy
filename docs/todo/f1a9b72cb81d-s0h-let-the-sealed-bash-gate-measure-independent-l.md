---
id: f1a9b72cb81d
kind: refactor
title: S0H Let the sealed Bash gate measure independent layer artifacts
seq: 3243
status: todo
priority: p0
labels:
    - superseded-plan
created: 2026-11-18T01:26:31.650866Z
sprint: 575
sprint_id: 65ce85f5-87c0-5863-bf55-62de86f5df36
sprint_title: 'Layer separation: Classic <- Bash# <- Yoke import-graph delineation'
---

Sprint575 reviewed gate adaptation, from umbrella final split-artifact source review. Reuse current S0G make-free harness/evidence/image builders; do not create a new verifier. Pure A refuses all Bashsharp invocation options, so old single-testee matrix passing --no-bashpp in Classic mode cannot measure A. Add only the minimum explicit artifact/invocation seam below. Default existing same-artifact behavior remains unchanged until caller selects split mode.

1. scripts/build-conformance-image.sh: accept an explicit testee executable for split-layer runs, stage it at the existing baked path with sibling .real payload if it has the existing native launcher layout. Never silently rebuild C cmd/bash when an external A/B/C artifact is selected. Preserve fixture fetch/frozen86 inventory, harness build, runtime-image digest/UID/network/readonly/tmpfs contract. Build/test Go only inside the official image; host adapters may only stage artifacts/fixtures and execute sealed runtime.
2. tools/bash53suite: add a narrow Classic-artifact invocation option. For A, mode0 fixtures AND mode probe receive no --no-bashpp or other Bashsharp flag; preserve the actual Classic proof (shell marker printed, language declaration rejected). Reject inconsistent A/mode1 selection. B/C mode1 keep --bashpp, active-language probe and existing behavior. Preserve BASH53_BASHPP versus -bashpp-mode agreement, environment filtering, all named fixtures, timeout and verdict semantics.
3. scripts/test-bash-container.sh: pass the explicit artifact and invocation choice through existing image builder/harness; keep artifact-specific evidence output so B and C do not overwrite one mode file. Record actual selected testee binary identity, source commit and fixed lower module pins without mislabeling them as the old C/sh source fields. Existing provenance fields remain truthful for legacy path. Prefer a small explicit operator/build-stage provenance input tied to the existing binary checksum over a new manifest framework. No guessed commits/pins or private host paths in public evidence.
4. Extend the existing fake-OCI mode regression for explicit A no-flag path, invalid A/mode1 selection and artifact forwarding while retaining all current selector-loss/invalid-selector/failure assertions. Focused harness argument/probe tests only where meaningful. No fixtures or baseline expectation weakening, no new skips, no timeout/threshold/allowlist changes, no product/runtime or dependency changes, no KB.md/root/sibling edits. Public POSIX regression and licensed certification remain distinct.

All authoritative Go builds/tests and mode checks run through manager's digest-locked official image. No native Go build/test/run or foreign-suite execution on development box; static formatting and script parsing only. Work in isolated workspace, no pushes/branch switches. Commit Sprint575/Story/Story-ID and actual agent. Report exact optional inputs and defaults, changed files, source-preservation boundaries, SHA and pending official gates. Root DAG wiring and actual A/B/C86 runs follow after their accepted artifacts exist; do not claim them from this helper slice.
