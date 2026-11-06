---
id: d66818b129d0
kind: feature
title: 'S583.8 Agents and fleet: catalog circle membership, delegation and spawn checks'
seq: 3139
status: todo
priority: p1
labels:
    - boundary
    - fleet
created: 2026-11-06T21:45:56.387164Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Agents and fleet membership. Fleet catalog entries (agent, model, tool, person, whois, lexicon; yoke/pkg/fleet) carry circle sets; the existing org overlay ring (fleet/catalog.go) maps to org: circles; peers/hosts get theirs from pairing records. chat/invoke, delegate, coach, foreman, pair, judge, supervise refuse to spawn or delegate to a non-member and pass only BASHY_CIRCLE of the session to children.
Depends on: S583.2.
Acceptance: delegation to a non-member agent refused; child sees exactly one circle; agent list/whois filtered to the active circle.
