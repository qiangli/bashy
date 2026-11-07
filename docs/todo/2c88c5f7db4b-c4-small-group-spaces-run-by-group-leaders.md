---
id: 2c88c5f7db4b
kind: task
title: C4 Small-group spaces run by group leaders
seq: 3172
status: todo
priority: p1
labels:
    - class
created: 2026-11-07T21:11:18.500395Z
sprint: 587
sprint_id: 6ed807a4-dcaf-50c6-9f4c-843d37538a1e
sprint_title: 'Class satellite for Sunday: students pair, share by choice, small groups, shared LLM and Klatch (existing features only)'
---

Each group leader runs the Bashy collaboration examples on their own outpost (examples/whiteboard and examples/brainstorm: register.sh, then serve) and the meet service for text rooms, publishes them with outpost apps add NAME --url http://127.0.0.1:PORT, and gives access through each app's own invite links (whiteboard: private named board per group with per-person invites; brainstorm: named session, open to anyone with the link). Write the leader runbook and verify one group end to end with test accounts. Limitation: satellite-level sharing by a student is not available (host ownership), so access control is the app's own. KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.

Simplest fallback, decided at the dry run: the operator runs whiteboard and brainstorm once on the operator host with one named board/session per group plus a class board, and meet with one room per group; per-participant whiteboard invite links. This avoids depending on each leader host.
