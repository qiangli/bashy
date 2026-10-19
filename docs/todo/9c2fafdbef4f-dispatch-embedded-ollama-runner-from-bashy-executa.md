---
id: 9c2fafdbef4f
kind: feature
title: Dispatch embedded Ollama runner from Bashy executable
seq: 1918
status: done
priority: p1
labels:
    - sphere
created: 2026-10-13T16:46:56.218352Z
weave: 96
assignee: codex-gpt6-luna
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T16:56:45.778445Z
closed_by: s436-codex-manager
---

After signed runner recipe exists, wire Bashy executable early runner dispatch so the Yoke Ollama server re-exec path (os.Executable() runner ... ) launches the embedded verified runner and preserves argv, exit status, signals and shutdown. Source protocol is official ollama/ollama v0.21.2; do not execute unverified payload or add a PATH fallback. Focused behavior tests, no dev-host native inference/full suite. Worker commit with story trailers, no push.
