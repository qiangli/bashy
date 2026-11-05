---
id: 4fd0a1b6157c
kind: feature
title: H2 Offer, accept, decline and status records and verbs
seq: 3070
status: todo
priority: p1
labels:
    - clean-baseline
    - handoff
created: 2026-11-05T00:54:34.611834Z
sprint: 570
sprint_id: 951dfb3f-3306-583d-8a93-02df8ed4464b
sprint_title: 'Live handoff: offer/accept transfer of sprints, stories and claims'
---

Depends on H1. Scope: bashy handoff offer <ref> --to <who> [-m brief] creates a durable offer (snapshot + brief) and delivers it through existing MB/bus/inbox human; accept/decline (-m why)/status; states offered, delivered, accepted, declined, expired; one live offer per ref; offers expire. Records actor and represented user. Sugar: sprint handoff N --to WHO creates an offer; sprint take N --offer ID accepts it; without --to, existing behavior unchanged. Acceptance: state machine and records tested; audit entries on the sprint/story thread; humans and agents use the same verbs and authorization.

Plan: docs/plan-live-handoff.md (motivation docs/retro-s562-manager-handoff.md). Planning record only; no implementation started. Local, additive bashy feature: reuse existing sprint/todo/claim/weave records and MB/bus/inbox human; no new transport or identity model. Vocabulary per Sprint 568 (actor, represented user, durable destination, delivery ack vs acceptance); identical authorization for humans and agents. Delivery commits carry Sprint, Story and Story-ID trailers.
