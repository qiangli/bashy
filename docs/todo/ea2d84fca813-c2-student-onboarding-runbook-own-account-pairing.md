---
id: ea2d84fca813
kind: task
title: 'C2 Student onboarding runbook: own-account pairing, mutual membership, satellite member, share by choice'
seq: 3170
status: done
priority: p0
labels:
    - class
created: 2026-11-07T21:11:18.202023Z
assignee: claude-opus5.5-s587
sprint: 587
sprint_id: 6ed807a4-dcaf-50c6-9f4c-843d37538a1e
sprint_title: 'Class satellite for Sunday: students pair, share by choice, small groups, shared LLM and Klatch (existing features only)'
closed: 2026-11-07T23:44:19.424073Z
closed_by: claude-opus5.5-s587
---

Steps for a student with their own cloudbox account: (1) install Bashy (curl|sh or irm|iex); (2) pair the outpost with the student's OWN account on the main cloudbox (default outpost register with a code the student mints for themselves), so the student owns the host and nothing is reachable by anyone else unless shared; (3) accept the operator's member invitation (membership is mutual: each party on the other's member list, automatic when accepted); (4) sign in to the class satellite as a member (delegated satellite login) to use the class apps; (5) share only what they choose (an app, ollama/GPU, podman) with trusted members as the host owner. Sharing is a trusted relationship: minimal system info and online status visible to sharing partners is acceptable for debugging; the dry run records exactly what the operator sees. Optional hardening for students who want it: outpost builtins set --service podman=off --service ollama=off --service ollama-pool=off --service files=off --service meet=off --shell off --desktop off --clipboard off. Do not pair student hosts with the class satellite itself (that makes the satellite owner the host owner). KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.

What a sharing partner sees today (checked in code 2026-11-07): host name and title (alias), online status and last seen, and the names of the shared apps. Not yet visible to partners: CPU/GPU/memory/disk, OS/platform/arch, Bashy/outpost versions, IP (outpost reports all but IP to the host owner). For Sunday: when debugging, the student runs outpost status (or bashy doctor) and pastes the output in Klatch. Code follow-up filed separately.

## Progress (2026-11-07)

Runbook written as a generic kb page (repo kb and kb/defaults): join-someone-s-satellite-as-a-member-and-share-your-machine-by-c. Member steps: install; pair with your OWN account (bashy login --code); mutual membership; sign in to the satellite (delegated login, any active account); share by choice (outpost apps add + share from the Hosts page); optional hardening; paste outpost status when debugging. Owner steps: add members, share class apps, never hand out pairing codes. Checked: CLI flags via --help; delegated member sign-in and share-then-access were verified live earlier with a second account; ownership and mutual membership come from code and operator rules. Remaining for acceptance: a full run with a test account (in C6).
