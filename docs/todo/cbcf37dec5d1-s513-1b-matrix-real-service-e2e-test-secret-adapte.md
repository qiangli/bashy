---
id: cbcf37dec5d1
kind: test
title: S513.1b Matrix real-service E2E test secret adapter and repair
seq: 2738
status: todo
priority: p0
labels:
    - deferred
    - operator-stop
    - operator-stopped
created: 2026-10-27T17:17:29.268978Z
---

Follow-up to submitted 16b9bc2b2dfd commit 6268b286; manager independent Python unit gate passed, but real Matrix E2E on claimed novidesign stops at first bashy secret set because test user lacks Cloudbox vault token. Implement BOUNDED test-only synthetic secret adapter in examples/matrix-chat/e2e.py or its test harness (for example private PATH shim intercepting only bashy secret commands and dispatching all other bashy verbs to genuine binary). It must use synthetic per-run credentials, restrict to the isolated E2E process/data, never affect production config/provisioning path or expose secrets in logs, and fail closed if signed tuwunel/service absent. Run actual signed service E2E on novidesign: two users login, invite/join, text+image, idempotent setup, restart persistence and media bytes. Coordinate Cloudbox shim contract with S513.4a worker. Keep edits scoped to examples/matrix-chat and focused tests; no unrelated files, no containers, no paid infra. Read CLAUDE.md. Commit in isolated workspace with injected identity/trailers; report exact commands, outputs, commit and remaining client HTTPS criteria. Manager gates/merges; do not accept original story based on wrapper submission.
