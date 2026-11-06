---
id: 7f785ad19fc2
kind: task
title: Freeze and gate the v1.0.0-rc.25-dev candidate
seq: 3130
status: todo
priority: p0
labels:
    - release
created: 2026-11-06T20:10:28.002012Z
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
---

After the pin reconciliation story lands: freeze one candidate commit and build it once from the v1.0.0-rc.25-dev tag (release workflow; promotion reuses the tested bytes, never rebuilds). Required evidence, each tied to the exact candidate SHA:
- make test-bash-container: 86/86, zero failures/timeouts/skips (authoritative hermetic gate).
- GOWORK=off make test green; CGO_ENABLED=0 Windows cross-build; e2e dispatch gate on the 3-OS CI matrix.
- Licensed 493-TP shell gate (make test-bash-system on the licensed native host, outside public CI; record pass/fail counts only, never suite content).
- govulncheck report attached (currently report-only; list counts per module).
- Signing status line from the candidate notes (signed=true|false per platform).
Reuse passing evidence only when tied to the same candidate; a pre-existing failure is tracked separately, never reported as passing.

Added gates (sprint 580 plan, 2026-11-06), each tied to the candidate SHA:
- make license-gate (blocking per docs/release-runbook.md).
- test-bash locale pinned and stated in the receipt (see 9e8271251090: 83/86 with LANG unset).
- Commit-pinned release/tier-verdicts/<sha>.json for the candidate (promote.yml requires it).
- Layer ratchets from Sprint 575 S0 (72c55602dd2e, script/layer-gate.sh, GOWORK=off) green; pre-existing failures listed in the K1 ledger.
- E1 Klatch E2E (a8c4da9c) run on the candidate; BLOCKED legs go to the ledger.
- scripts/verify-signatures.sh on the native QA runner when reachable; otherwise record the signing status line only.
Schedule: freeze and tag Saturday afternoon; build once.
