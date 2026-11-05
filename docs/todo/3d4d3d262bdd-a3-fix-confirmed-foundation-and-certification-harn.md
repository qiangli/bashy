---
id: 3d4d3d262bdd
kind: bug
title: A3 Fix confirmed foundation and certification-harness defects
seq: 3036
status: done
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T02:24:39.525115Z
assignee: claude-opus5.5
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
closed: 2026-11-04T07:47:55.126971Z
closed_by: claude-opus5.5
---

Depends on A2 findings; repair incrementally. This campaign includes required A product fixes, not investigation only. Apply smallest causal shell/coreutils or harness correction, preserve test meaning, protect previously green behavior and add focused regressions. Address applicability/manual dispositions through the proper authority, never raw-result rewriting. Existing large/risky-change review and licensed-run requirements remain. Acceptance: confirmed in-scope defects fixed and affected original cases rerun; no skips or timeout increases merely to obtain green.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
