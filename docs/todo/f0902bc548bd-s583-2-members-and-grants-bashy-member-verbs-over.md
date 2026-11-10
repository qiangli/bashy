---
id: f0902bc548bd
kind: feature
title: 'S583.2 Members and grants: bashy member verbs over the Cloudbox list, circle check, owner = paired email'
seq: 3133
status: done
priority: p0
labels:
    - boundary
    - core
created: 2026-11-06T21:45:55.476745Z
assignee: codex-gpt6-sol
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control (KISS): owner by default, circles from the member list, checked where data leaves'
closed: 2026-11-10T12:34:15.188622Z
closed_by: claude-opus5.5-s606
---

Members and grants, the small core. KISS rescope 2026-11-09. Depends on: S583.1; client codes against the Cloudbox members wire (ticket a99e88ac) with a fixture until it ships.
- yoke/pkg/boundary: Member{Email, Name, Status, Circle}; a members client for GET /api/v1/members (Bearer, members:read) using the host's account credential; a short-TTL cache (offline: last known list); Allowed(email, grants) where a grant is an email or a circle and only active members match.
- yoke/pkg/principal: the owner is the paired account email (fallback "local" when unpaired, as today).
- One agent-first verb over the Cloudbox member list (no web UI step): `bashy member ls [--circle C]`, `bashy member invite EMAIL --circle C`, `bashy member accept ID`, `bashy member set EMAIL --circle C`, `bashy member rm EMAIL`, `bashy member of` (who lists me), `bashy member check EMAIL` (member? circle? why not). --json on every verb, --dry-run on writes. Circle is a flag, not a second noun.
- No circles.yaml, no BASHY_CIRCLE, no store partitions.
Acceptance: unit tests for list parsing, cache/offline, Allowed matrix (member, wrong circle, inactive, unknown, removed); behaviour unchanged for unpaired hosts; yoke and bashy GOWORK=off targeted tests green.
