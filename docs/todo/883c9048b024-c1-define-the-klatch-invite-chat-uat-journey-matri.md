---
id: 883c9048b024
kind: requirement
title: C1 Define the Klatch invite/chat UAT journey matrix
seq: 3042
status: todo
priority: p0
labels:
    - clean-baseline
created: 2026-11-04T02:24:40.665614Z
sprint: 563
sprint_id: 282b20a5-f15c-55eb-a314-eef9580e19d1
sprint_title: 'Product UAT: clean-install user journeys and defect closure'
---

Define the finite journey matrix for Klatch group chat as a Bashy feature for every Bashy user: any cloudbox user can be invited to the chat on the host owner's satellite (needs only a cloudbox account and a browser); the host running Klatch always has outpost. Journeys J1-J10 with input, expected outcome, failure checks and platforms (W native Windows elephant, M macOS dragon, L claimed Linux host): J1 clean install + pair; J2 native Klatch service from the pinned signed release; J3 media tools from Bashy only; J4 publish with login + trusted identity, survives restart; J5 host owner setup by one Bashy command; J6 invited member single sign-on into the team topic; J7 host reboot; J8 satellite move zoo to zoo-aws; J9 agent coordination (mb/inbox/sprint); J10 shapeable-ready rebuild from pinned source. Klatch vocabulary is host owner/member/team; cohort words never appear in the product. psic2027 members are the real-user UAT cohort, not scope. Acceptance: matrix and P0-P3 priorities recorded in the plan and agreed by the operator.

Execution plan: docs/plan-s563-master-execution.md (campaign: docs/plan-clean-baseline-campaign.md). KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
