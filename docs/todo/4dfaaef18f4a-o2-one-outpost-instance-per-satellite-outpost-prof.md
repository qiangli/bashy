---
id: 4dfaaef18f4a
kind: feature
title: 'O2 One outpost instance per satellite: outpost-profile up/down/status'
seq: 3059
status: done
priority: p0
labels:
    - outpost
created: 2026-11-04T06:51:50.122392Z
assignee: claude-opus5.5-s565
sprint: 565
sprint_id: 45140e29-11a7-5dac-b634-da49c692c9f6
sprint_title: Satellite-less single-node outpost by default; join and switch between satellites
closed: 2026-11-07T09:35:12.545742Z
closed_by: claude-opus5.5-s565
---

Simultaneous multi-satellite support, KISS: one outpost process per extra satellite instead of a multi-satellite daemon. Extend bashy/examples/outpost-profile/outpost-profile.bsh (no outpost change expected):
- up <name>: run the profile as its own outpost instance: an isolated config and cache root, its own admin port, LAN sshd/discovery/mesh/desktop/clipboard/shell turned off through existing agent.json settings, chosen app entries (including the SSO secret) copied from the main outpost so every instance proxies to the same local backend, and a Bashy service unit outpost-<name> so it survives a reboot.
- down <name>: stop and remove that unit; the main outpost and the other instances keep running.
- status: one line per instance: name, satellite, host name, admin port, running.
Cluster mode, the ollama pool, peer mesh and LAN ssh stay on the main instance only. The same email arriving from two satellites is one Klatch account (MVP). If S1 finds a collision that config cannot turn off, make the smallest outpost fix as a separate, red/green-tested change (S2b). Tests: outpost_profile_test.go with a fake outpost, covering up/down/status, the toggles, the app copy and the refusal of an unknown profile. Target: ready before Sunday 2026-11-08. KISS/MVP: reuse existing code and harnesses; smallest causal change. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
