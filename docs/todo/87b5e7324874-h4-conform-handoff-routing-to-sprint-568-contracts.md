---
id: 87b5e7324874
kind: feature
title: H4 Conform handoff routing to Sprint 568 contracts
seq: 3072
status: todo
priority: p2
labels:
    - clean-baseline
    - handoff
created: 2026-11-05T00:54:34.944646Z
sprint: 570
sprint_id: 951dfb3f-3306-583d-8a93-02df8ed4464b
sprint_title: 'Live handoff: offer/accept transfer of sprints, stories and claims'
---

Depends on Sprint 568 D3 + D4 and Sprint 563 F9-F9d. Scope: deliver offers and acknowledgements through Sprint 568's destination and delivery contracts (ambiguous-alias resolution, offline queue, retry, dedup, delivery acknowledgement distinct from acceptance, revocation); remove any interim local addressing workaround. Acceptance: no separate local addressing scheme remains; the scenarios in S568 acceptance items 3 and 6 pass for handoff.

Plan: docs/plan-live-handoff.md (motivation docs/retro-s562-manager-handoff.md). Planning record only; no implementation started. Local, additive bashy feature: reuse existing sprint/todo/claim/weave records and MB/bus/inbox human; no new transport or identity model. Vocabulary per Sprint 568 (actor, represented user, durable destination, delivery ack vs acceptance); identical authorization for humans and agents. Delivery commits carry Sprint, Story and Story-ID trailers.
