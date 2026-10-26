---
id: 159c0595d939
kind: bug
title: 'TestCommandDocSourceNames red on bashy main: Names() now includes ''transcribe'' but the test''s want list does not'
seq: 2725
status: todo
priority: p2
created: 2026-10-26T06:18:51.437621Z
---

Seen 2026-10-25 by s507-manager on bashy main 7c02a95f (and e4dbdbfd): go test ./internal/agentos -run TestCommandDocSourceNames -> want list lacks 'transcribe' (added with the transcribe command docs, e.g. d60e3f00 lineage). Fix: add 'transcribe' to the want list in internal/agentos/commanddocs_test.go if the doc is intended. Not caused by Sprint 507.
