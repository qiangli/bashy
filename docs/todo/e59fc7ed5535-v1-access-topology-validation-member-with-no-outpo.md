---
id: e59fc7ed5535
kind: test
title: 'V1 Access-topology validation: member with no outpost, unpaired outpost on the LAN, outpost on an owned or shared satellite'
seq: 3162
status: todo
priority: p1
labels:
    - klatch
    - outpost
created: 2026-11-07T08:33:56.84067Z
sprint: 585
sprint_id: 73032105-b38f-5cd7-8f47-a732dfee103f
sprint_title: Klatch v1.2.7 rebuild and end-to-end tests
---

Validate the three access topologies on the released Bashy with Klatch v1.2.7 (from story 295f6aecae48). Each cell records pass, fail or BLOCKED with raw evidence and the exact Bashy, outpost and Klatch identities. Reuse tests/klatch-e2e/run.py (E1 a8c4da9cf37d) and the C4 journeys; add only the missing legs.

(a) No satellite, no outpost: an invited member with only a cloudbox account and a browser opens the shared Klatch tile on the owner's satellite, signs in once with SSO, lands in the team topic and exchanges messages with the owner. Failure checks: a member the app was not shared with is refused; a forged or unsigned identity is refused (401). Existing evidence: J6 passed 2026-11-04 on the shaped container only; it has not been run on a release. Missing harness leg: run.py drives only the owner's browser, so add a second member browser identity.

(b) No satellite, one outpost: a fresh unpaired outpost starts, a Klatch app is added, and Klatch is reachable on this host and from another host on the same LAN. Satellite-only features report that they are unavailable instead of failing. Code inspection 2026-11-07: an unpaired daemon boots and app CRUD works (outpost TestE2E_StatusUnpaired and TestE2E_AppCRUD); the main and admin listeners bind loopback only (conf DefaultLocalAddr 127.0.0.1:0, DefaultAdminAddr 127.0.0.1:17777); the matrix tunnel is started even when unpaired and dials the default 127.0.0.1; no LAN path to apps was found. Expected outcome: the local leg passes and the LAN leg fails. A failure is routed to Sprint 565 O1 (f7459c010953), not fixed here.

(c) Outpost paired with a satellite, owned and shared: publish Klatch with login and trusted identity; it stays reachable after an outpost restart and after the 12 h credential rotation (F1); repeat with the outpost paired to a satellite owned by another account. Failure checks: anonymous access gets 401; the app entry is still present after the rotation.

Acceptance: every cell recorded on the release, and every failure routed to an owning story. Hosts are named by role only; satellite URLs and credentials come from the environment or a profile and are never committed.
