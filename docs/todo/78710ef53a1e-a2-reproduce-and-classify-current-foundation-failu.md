---
id: 78710ef53a1e
kind: test
title: A2 Reproduce and classify current foundation failures
seq: 3035
status: assigned
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T02:24:39.368634Z
weave: 9
assignee: muse-conductor
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
---

Depends on A1. Run the agreed current-baseline suites under their supported containment and finite bounds. Distinguish product defects, setup failures, applicability and manual review. Retain raw verdicts, denominators and hashes. Consult 560/3059 and 3063 receipts only where relevant; do not import historical verdicts as current results. Licensed runs require the exact supported approval. Acceptance: all required inventory entries have an execution result or explicit blocker with cause and next action.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

## Manager brief (revised 2026-11-03 per operator: no new baseline run)

Baseline = the migration-validated frozen tree: bashy main is identical to `5a4bd03f` outside docs (S557 story `bf7624a5c2f2` acceptance in umbrella `docs/todo/`). Do NOT rerun lanes whose inputs are unchanged — reuse that evidence, labelled "unchanged-input reuse" with its run id/source, never as a fresh result. No licensed run in A2: the licensed 493-TP shell arm and Profile D base run happen once, on the final candidate in A4. The licensed 493/493 run of S557 story 233 was on `c3d3b95f`, which is NOT current main; cite it only as historical reference. Do not modify $HOME/projects/poc/dhnt (archived) or poc/bashy. All commands GOWORK=off.

Work (read-only collection, no fixes):
1. Baseline evidence table from the S557 acceptance record: Bash 5.3 86/86 Linux+macOS (run 41705603801), Windows 84/86 (run 41708335900: baseline glob-test, printf timestamp), module lanes and their known Gotreesitter/Yamux failures.
2. Issue list = every known non-pass in scope of the Sprint 341 base profile and the A1 inventory, from: the S557 acceptance record and its unchecked child rows, Sprint 560 follow-ups, the preserved repair branches (`s557-repair-stack-quarantined`, `c3d3b95f` ancestry) and harness-kit dispositions. For each: source, affected lane, product defect | setup | applicability | pre-existing/advisory, whether a preserved patch exists, and next action. Out-of-scope items (optional targets, advisory suites) are listed as such, not queued for A3.
3. A fresh run only where no unchanged-input evidence exists for a required lane; name the gap before running.

Deliver `docs/s561/a2-results.md`. Raw logs stay out of git. Commit only that file with trailers Sprint: #561, Story: A2, Story-ID: 78710ef53a1e. Do not push.
