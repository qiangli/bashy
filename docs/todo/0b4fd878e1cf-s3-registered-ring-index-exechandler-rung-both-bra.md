---
id: 0b4fd878e1cf
kind: task
title: 'S3: registered ring index + ExecHandler rung (both branches) + front door + agentic native path'
seq: 279
status: done
priority: p0
created: 2026-09-14T19:42:32.814752Z
assignee: voussoir
sprint: 179
closed: 2026-09-14T20:13:38.392453Z
closed_by: voussoir
---

internal/agentos/registered.go: index/cache, reservedCommandName, registeredHandler (cert-profile exclusion), runRegisteredFrontDoor; dispatch() + isFrontDoorInvocation; command hidden no-shim alias; agenticCommand registered+registry.Names() native; resolveCmd; wireLexicon; inspect paths; doctor. Gate: go test ./internal/agentos/
