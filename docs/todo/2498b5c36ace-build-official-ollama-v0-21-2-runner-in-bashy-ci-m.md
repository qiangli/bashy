---
id: 2498b5c36ace
kind: feature
title: Build official Ollama v0.21.2 runner in Bashy CI matrix
seq: 1917
status: done
priority: p1
labels:
    - sphere
created: 2026-10-13T16:46:02.140029Z
weave: 95
assignee: codex-gpt6-astra
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T16:56:41.490013Z
closed_by: s436-codex-manager
---

Source qualification: Yoke docs/validation/ollama-runner-source-2018.md at 5e8c4687. In Bashy builds/ pin exact official ollama/ollama v0.21.2 SHA 590109c8352e8d5a6206e8909b518a54a2b0a7b8, separate from Yoke fork. Generate a fork/build recipe for native Linux amd64/arm64, Darwin amd64/arm64, Windows amd64 using unpatched upstream cmd/runner, full Git source (nested headers), GOWORK=off, cgo. Include deterministic gzip, signed release artifact contract and renderer/guard tests. Extend existing draft branch carefully, but do not publish or claim native/security gates; no dev-host heavy build/container/full suite. Worker commits with trailers, no push.
