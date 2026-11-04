---
id: 12861d60bc4f
kind: test
title: A4 Verify one certification-ready foundation candidate
seq: 3037
status: todo
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T02:24:39.679625Z
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
---

Depends on A3 and completed applicability/manual review. Freeze exact candidate and rerun required profile with consistent toolchain/providers and complete denominators. Compare previous green coverage and retain all evidence. Prepare minimal certification handoff with identities, reproducible commands and residual advisory findings. Acceptance: all required results acceptable under the agreed profile, no unexplained failures or unresolved/manual items; independent review. Do not claim formal certification or publish an official release.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
