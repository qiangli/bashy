---
id: baa30d434273
kind: feature
title: H3 Atomic transfer on accept
seq: 3071
status: todo
priority: p1
labels:
    - clean-baseline
    - handoff
created: 2026-11-05T00:54:34.778047Z
sprint: 570
sprint_id: 951dfb3f-3306-583d-8a93-02df8ed4464b
sprint_title: 'Live handoff: offer/accept transfer of sprints, stories and claims'
---

Depends on H2. Scope: accepting an offer moves together or not at all: conductor lease (sprint), named claims (epoch +1), story/todo assignees, the sprint-role resolution so role-addressed messages reach the new owner, and the snapshot's outstanding obligations. The outgoing party stays accountable until the accept; no window without an owner. Acceptance: all-or-nothing under injected failure; rerun-safe; both parties and the thread notified; outgoing party relieved only after the accept.

Plan: docs/plan-live-handoff.md (motivation docs/retro-s562-manager-handoff.md). Planning record only; no implementation started. Local, additive bashy feature: reuse existing sprint/todo/claim/weave records and MB/bus/inbox human; no new transport or identity model. Vocabulary per Sprint 568 (actor, represented user, durable destination, delivery ack vs acceptance); identical authorization for humans and agents. Delivery commits carry Sprint, Story and Story-ID trailers.
