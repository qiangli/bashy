---
id: d66818b129d0
kind: feature
title: 'S583.8 Agents and fleet: Cloudbox member circle sync (guest/family/friend/team), catalog membership, delegation and spawn checks'
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

Agents and fleet membership. Fleet catalog entries (agent, model, tool, person, host, whois, lexicon; yoke/pkg/fleet) carry circle sets (`--circle` flag on `bashy person/agent/host add/set`). Unify with the Cloudbox account member list: member emails with `circle` (formerly `role`: `guest` | `family` | `friend` | `team`) and attached satellites/accounts (yoke/pkg/accounts) populate person and paired-host circle memberships without duplicate manual lists in circles.yaml; the synced org overlay ring (fleet/catalog.go) maps to `team` (or `team:<name>`). chat/invoke, delegate, coach, foreman, pair, judge, and supervise refuse to spawn or delegate to a non-member and pass the ceiling-locked BASHY_CIRCLE of the session to children.
Depends on: S583.2.
Acceptance: Cloudbox member circles (guest/family/friend/team) resolve into fleet membership; delegation to a non-member agent refused; child sees and is locked to one circle; agent/person/whois list filtered to the active circle.
