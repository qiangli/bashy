---
id: f0902bc548bd
kind: feature
title: 'S583.2 Members and grants: Cloudbox member list client, circle check, owner = paired email'
seq: 3133
status: todo
priority: p0
labels:
    - boundary
    - core
created: 2026-11-06T21:45:55.476745Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control (KISS): owner by default, circles from the member list, checked where data leaves'
---

Members and grants, the small core. KISS rescope 2026-11-09. Depends on: S583.1; client codes against the Cloudbox members wire (ticket a99e88ac) with a fixture until it ships.
- yoke/pkg/boundary: Member{Email, Name, Status, Circle}; a members client for GET /api/v1/members (Bearer, members:read) using the host's account credential; a short-TTL cache (offline: last known list); Allowed(email, grants) where a grant is an email or a circle and only active members match.
- yoke/pkg/principal: the owner is the paired account email (fallback "local" when unpaired, as today).
- bashy circle ls [--circle C] [--json] and bashy circle check EMAIL (member? circle? why not).
- No circles.yaml, no BASHY_CIRCLE, no store partitions.
Acceptance: unit tests for list parsing, cache/offline, Allowed matrix (member, wrong circle, inactive, unknown, removed); behaviour unchanged for unpaired hosts; yoke and bashy GOWORK=off targeted tests green.
