---
id: 518cc2ed7b07
kind: bug
title: genie with a door-* model never starts the model door and stalls silently at llm.requested
seq: 415
status: todo
priority: p1
labels:
    - genie
    - llm
created: 2026-10-08T21:45:51.413771Z
sprint: 379
sprint_id: 908c2ac2-e7bc-55fe-86bd-d25046ac4684
sprint_title: 'bashy 1.0.0 feature list: bash + Bash# + Yoke'
---

Found live on Dragon 2026-10-08 (S379 conductor): with no door running, 'bashy genie -m door-codex-gpt-5.5 ...' reached llm.requested and waited until timeout with no output; after 'bashy llm up' the same probe finished in 14s. The local-model recipe runs 'llm up' (genie lib/model-server.bsh); the external-model path (internal/agentos genie_external.go) did not, although door-* registry entries point base_url at the door (127.0.0.1:24556). Fix: genieExternal.prepare starts the door (broker.EnsureUp) when the base_url is this host's door, fails loudly when it cannot.
