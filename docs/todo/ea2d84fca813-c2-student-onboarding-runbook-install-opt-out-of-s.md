---
id: ea2d84fca813
kind: task
title: 'C2 Student onboarding runbook: install, opt out of shared services, pair, verify metadata only'
seq: 3170
status: todo
priority: p0
labels:
    - class
created: 2026-11-07T21:11:18.202023Z
sprint: 587
sprint_id: 6ed807a4-dcaf-50c6-9f4c-843d37538a1e
sprint_title: 'Class satellite for Sunday: students pair, share by choice, small groups, shared LLM and Klatch (existing features only)'
---

Steps for a student with their own cloudbox account: install Bashy (curl|sh or irm|iex), turn off the services that would otherwise be reachable by the host's satellite owner (outpost builtins set --service podman=off --service ollama=off --service ollama-pool=off --service files=off --service meet=off --shell off --desktop off --clipboard off; applies live), then pair with a plain, UNSCOPED pairing code minted by the operator (scoped codes auto-share services across the scope; do not use them). Shell, desktop and LAN ssh stay behind the student's own OS password. Verify with a test account that the operator sees only system info and online status. Record the limitation: in the satellite record the paired host is owned by the satellite owner (student-owned pairing is follow-up work). KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.

Membership is mutual on cloudbox (each party on the other's member list; automatic when an invitation is accepted): the operator invites each student as a member and the student accepts before pairing; shares reach the student only after that. Verify both member lists in the dry run.
