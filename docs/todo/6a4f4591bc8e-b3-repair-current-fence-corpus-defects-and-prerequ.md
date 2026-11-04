---
id: 6a4f4591bc8e
kind: bug
title: B3 Repair current fence corpus defects and prerequisites
seq: 3040
status: todo
priority: p2
labels:
    - clean-baseline
created: 2026-11-04T02:24:40.239759Z
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
---

Depends on B2 findings. Fix smallest causal language/provider/fixture issues, retaining malformed-engine rejection, no-fallback and effect-boundary behavior. Use hermetic test helpers where possible. Route A causes to sprint A and C causes to sprint C with explicit cross-links; keep failing corpus acceptance pending until consumed fix passes. Acceptance: confirmed required corpus failures fixed and original cases rerun; no weakened assertions, blanket exclusions or unrelated architectural work.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
