---
id: 6a4f4591bc8e
kind: bug
title: B3 Repair current fence corpus defects and prerequisites
seq: 3040
status: assigned
priority: p2
labels:
    - clean-baseline
created: 2026-11-04T02:24:40.239759Z
assignee: codex-gpt6-astra
sprint: 578
sprint_id: 70ba48be-af89-5ab1-8a8d-42a15fc1ddbe
sprint_title: 'S562 follow-up: remaining fence corpus work'
---

Depends on B2 findings. Fix smallest causal language/provider/fixture issues, retaining malformed-engine rejection, no-fallback and effect-boundary behavior. Use hermetic test helpers where possible. Route A causes to sprint A and C causes to sprint C with explicit cross-links; keep failing corpus acceptance pending until consumed fix passes. Acceptance: confirmed required corpus failures fixed and original cases rerun; no weakened assertions, blanket exclusions or unrelated architectural work.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

Carried from S562 (operator-approved early close 2026-11-06); deferral is not PASS. Run/branch/evidence references: see existing story details and S562 board; no verified run or branch reference recorded.
