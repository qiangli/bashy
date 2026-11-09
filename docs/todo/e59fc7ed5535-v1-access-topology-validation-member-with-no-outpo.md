---
id: e59fc7ed5535
kind: test
title: 'V1 Access-topology validation: member with no outpost; outpost on an owned or shared satellite'
seq: 3162
status: todo
priority: p1
labels:
    - klatch
    - outpost
created: 2026-11-07T08:33:56.84067Z
sprint: 599
sprint_id: b38d7a89-cadd-58e2-8748-66b0e9c2f201
sprint_title: 'S585 follow-ups: native Klatch on the Windows test host, rc.30 end-to-end and UAT, local gates under 30 min'
---

Validate access topologies (a) and (c) on the released Bashy with Klatch v1.2.7 (from story 295f6aecae48). Each cell records pass, fail or BLOCKED with raw evidence and the exact Bashy, outpost and Klatch identities. Reuse tests/klatch-e2e/run.py (E1 a8c4da9cf37d) and the C4 journeys; add only the missing legs.

(a) No satellite, no outpost: an invited member with only a cloudbox account and a browser opens the shared Klatch tile on the owner's satellite, signs in once with SSO, lands in the team topic and exchanges messages with the owner. Failure checks: a member the app was not shared with is refused; a forged or unsigned identity is refused (401). Existing evidence: J6 passed 2026-11-04 on the shaped container only; it has not been run on a release. Missing harness leg: run.py drives only the owner's browser, so add a second member browser identity.

(b) No satellite, one outpost: DEFERRED by the operator on 2026-11-07 (not urgent). Not run in this story. Inspection notes and the owning story stay with O1 f7459c010953, which is now a standalone todo.

(c) Outpost paired with a satellite, owned and shared: publish Klatch with login and trusted identity; it stays reachable after an outpost restart and after the 12 h credential rotation (F1); repeat with the outpost paired to a satellite owned by another account. Failure checks: anonymous access gets 401; the app entry is still present after the rotation.

Acceptance: every cell recorded on the release, and every failure routed to an owning story. Hosts are named by role only; satellite URLs and credentials come from the environment or a profile and are never committed.
