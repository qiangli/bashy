---
id: 78710ef53a1e
kind: test
title: A2 Reproduce and classify current foundation failures
seq: 3035
status: todo
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T02:24:39.368634Z
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
---

Depends on A1. Run the agreed current-baseline suites under their supported containment and finite bounds. Distinguish product defects, setup failures, applicability and manual review. Retain raw verdicts, denominators and hashes. Consult 560/3059 and 3063 receipts only where relevant; do not import historical verdicts as current results. Licensed runs require the exact supported approval. Acceptance: all required inventory entries have an execution result or explicit blocker with cause and next action.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

## Manager brief (2026-11-03, conductor claude-opus5.5)

Phase 1 (start now, public lanes only; the A1 inventory lands in parallel and these lanes are certain to be in it). Baseline = bashy commit 3827a673, standalone. ALL commands with GOWORK=off and a clean PATH (`PATH=/bin:/usr/bin:$(dirname $(which go))`). Do not modify /Users/qiangli/projects/poc/dhnt (archived) or poc/bashy. Licensed VSC runs are NOT authorized: record them as "blocked: awaiting operator approval", do not attempt them.

Run once each, with the repo's existing bounds (no timeout increases, no skips):
1. `GOWORK=off go test ./...` (record per-package pass/fail/skip; note any test that is skipped by design).
2. `GOWORK=off make build` then `make test-bash-container` (authoritative 86-fixture gate). If no container runtime is available on this host, record that as a setup blocker with the exact error, then run native `make test-bash` as diagnostic only and label it non-authoritative.
3. Standalone cross-builds of `./cmd/bashy` and `./cmd/bash` with CGO_ENABLED=0 for linux/darwin/windows × amd64/arm64.
4. Advisory (not gating): `bashy dag suites.md yash` — record the number, label ADVISORY.

Deliver `docs/s561/a2-results.md`: one row per lane — exact command, start time, duration, expected vs executed denominator, PASS/FAIL/SKIP counts copied verbatim, and a classification for every non-pass: product defect | setup | applicability | manual review, with cause and next action. Keep raw logs OUT of git under `$HOME/.cache/bashy-s561/a2/` and list each log's sha256 in the doc. Do not fix anything in this story; defects go to A3. Do not import historical verdicts as current results.

Commit only docs/s561/a2-results.md with trailers Sprint: #561, Story: A2, Story-ID: 78710ef53a1e. Do not push.
