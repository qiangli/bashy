---
id: 5784dc5818eb
kind: test
title: H5 End-to-end handoff scenarios and skill doc
seq: 3073
status: todo
priority: p2
labels:
    - clean-baseline
    - handoff
created: 2026-11-05T00:54:35.110359Z
sprint: 570
sprint_id: 951dfb3f-3306-583d-8a93-02df8ed4464b
sprint_title: 'Live handoff: offer/accept transfer of sprints, stories and claims'
---

Depends on H3 (H4 for routed variants). Scope: scenario tests for agent->agent, human->agent, agent->human, agent-assisted human (actor + represented user), manager->worker story handoff, decline, expiry, and a role-addressed message after a sprint handoff; bashy handoff --help skill doc. Acceptance: all scenarios pass on the gate; the skill doc describes the verbs and the Sprint 568/563 relationships.

Plan: docs/plan-live-handoff.md (motivation docs/retro-s562-manager-handoff.md). Planning record only; no implementation started. Local, additive bashy feature: reuse existing sprint/todo/claim/weave records and MB/bus/inbox human; no new transport or identity model. Vocabulary per Sprint 568 (actor, represented user, durable destination, delivery ack vs acceptance); identical authorization for humans and agents. Delivery commits carry Sprint, Story and Story-ID trailers.
