---
id: df0cc6a1965c
kind: test
title: Prove official runner native protocol and no-egress on five hosted targets
seq: 1920
status: assigned
priority: p1
labels:
    - sphere
created: 2026-10-13T16:58:03.318786Z
weave: 97
assignee: codex-gpt6-astra
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
---

Build on Bashy main 0686f21f recipe from official ollama/ollama v0.21.2 pin. Replace unconditional builds/ollama/files/no-egress-test.sh hold with a real hosted native gate on Linux amd64/arm64, Darwin amd64/arm64, Windows amd64. Use a small licensed GGUF fixture with exact hash. From the gated executable prove health, model load, streaming completion, embedding, cancellation, process shutdown and no outbound network except configured loopback; record per-platform evidence and keep gate fail-closed on missing tools, fixture, bind, or runtime. Do not run native build/inference on Dragon. Commit worker changes/trailers, no push; conductor installs generated fork and runs hosted CI after review. Do not modify audit.yml/security gate.
