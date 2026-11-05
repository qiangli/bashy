---
id: 027d6ac69eb0
kind: feature
title: 'H1 Handoff snapshot: bashy handoff show <ref>'
seq: 3069
status: todo
priority: p0
labels:
    - clean-baseline
    - handoff
created: 2026-11-05T00:54:34.443186Z
sprint: 570
sprint_id: 951dfb3f-3306-583d-8a93-02df8ed4464b
sprint_title: 'Live handoff: offer/accept transfer of sprints, stories and claims'
---

Scope: one read-only command that prints what a handoff of sprint:N|uuid, story:<id>, todo:<id>, claim:<name> or run:<repo>#N would move: current owner/lease, stories with owners and states, linked weave runs and their state, claims held by the owner, last continuity, and the owner's unanswered inbound requests as obligations. Built only from existing data sources; --json with a versioned schema. Acceptance: finite schema documented; each ref kind covered by tests with fixture boards; works identically for a human and an agent owner; no writes.

Plan: docs/plan-live-handoff.md (motivation docs/retro-s562-manager-handoff.md). Planning record only; no implementation started. Local, additive bashy feature: reuse existing sprint/todo/claim/weave records and MB/bus/inbox human; no new transport or identity model. Vocabulary per Sprint 568 (actor, represented user, durable destination, delivery ack vs acceptance); identical authorization for humans and agents. Delivery commits carry Sprint, Story and Story-ID trailers.
