---
id: d254f562c61f
kind: feature
title: 'S583.12 Execution isolation: weave/booth/oci/contain circle mounts, env scrub, egress, git identity'
seq: 3143
status: todo
priority: p2
labels:
    - boundary
    - sandbox
created: 2026-11-06T21:45:56.997357Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Execution isolation. weave workspaces, sealed booths, oci/sandbox, contain, limit, and run mount only the active circle's partitioned stores (`~/.bashy/<tool>/circles/<circle>/`) and scrub other circles' env variables and credential files via secrets.ScrubAgentEnv. Sealed-booth egress allowlist (booth_sealed.go) and contain --net become per circle. weave per-run git identity (weave_impl.go) resolves from the circle's bound account/person email (team email in team circles, personal email in family/friend).
Depends on: S583.2, S583.10.
Acceptance: booth in team circle cannot read family/owner stores or env; commit identity matches the circle.
