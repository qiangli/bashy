---
id: 2c88c5f7db4b
kind: task
title: 'C4 Small-group spaces: leaders share their own apps with their group'
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

Each group leader runs the Bashy collaboration examples on their own outpost (examples/whiteboard and examples/brainstorm: register.sh, then serve; meet for text rooms), publishes them with outpost apps add NAME --url http://127.0.0.1:PORT --require-login --trust-cloud-identity, and as the host owner shares them with the members of their group (mutual members). Access goes through the cloudbox the leader's host is paired with. Fallback decided at the dry run: the operator hosts one board/session/room per group plus a class board on the operator host behind the class satellite, with per-participant whiteboard invite links. Verify one group end to end with test accounts. KISS/MVP: existing features only, no new code; gaps are covered by configuration, procedure or a recorded limitation. Hosts are described by role; no account addresses in records. Delivery commits carry Sprint, Story and Story-ID trailers.
