---
id: ea2d84fca813
kind: task
title: 'C2 Student onboarding runbook: own-account pairing, mutual membership, satellite member, share by choice'
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

Steps for a student with their own cloudbox account: (1) install Bashy (curl|sh or irm|iex); (2) pair the outpost with the student's OWN account on the main cloudbox (default outpost register with a code the student mints for themselves), so the student owns the host and nothing is reachable by anyone else unless shared; (3) accept the operator's member invitation (membership is mutual: each party on the other's member list, automatic when accepted); (4) sign in to the class satellite as a member (delegated satellite login) to use the class apps; (5) share only what they choose (an app, ollama/GPU, podman) with trusted members as the host owner. Sharing is a trusted relationship: minimal system info and online status visible to sharing partners is acceptable for debugging; the dry run records exactly what the operator sees. Optional hardening for students who want it: outpost builtins set --service podman=off --service ollama=off --service ollama-pool=off --service files=off --service meet=off --shell off --desktop off --clipboard off. Do not pair student hosts with the class satellite itself (that makes the satellite owner the host owner). KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.
