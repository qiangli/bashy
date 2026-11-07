---
id: 235472ea4ea8
kind: test
title: C6 Dry run before class with 2-3 test accounts
seq: 3174
status: done
priority: p0
labels:
    - class
created: 2026-11-07T21:11:18.801542Z
assignee: claude-opus5.5-s587
sprint: 587
sprint_id: 6ed807a4-dcaf-50c6-9f4c-843d37538a1e
sprint_title: 'Class satellite for Sunday: students pair, share by choice, small groups, shared LLM and Klatch (existing features only)'
closed: 2026-11-07T23:44:21.162975Z
closed_by: claude-opus5.5-s587
---

Run C2-C5 end to end with 2-3 test accounts on separate machines (one as a group leader) the day before class: pair, opt-out verified, Klatch class/1:1/group, one group space, one LLM call from an agent. Record results and any fixes to the runbooks; anything that needs code goes to follow-up. KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.

## Result (2026-11-07)

Operator: sandbox e2e with fake accounts is good enough for this run. Sandbox e2e (members scenario, private satellite repo dev 600bfad): one throwaway satellite in podman, fake example.test owner + 3 members; PASS 15/15 twice on an ephemeral droplet (destroyed), two-satellite regression 16/16. Bashy/outpost rc.27 release; satellite-member from bashy main. Verified: own-account pairing via satellite-member (host owned by the member); privacy by default (unshared member and the satellite owner refused); sub-group sharing (shared member reaches the board with their own identity); shared Klatch (owner + 3 members SSO as separate accounts, team topic holds members, members connected one to one). Not covered here: the LLM call (tracked in C5), per-group Klatch topics and an actual message (C3).
