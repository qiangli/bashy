---
id: 2034ddd1f960
kind: task
title: B1 taught surface + e2e for CRUD verbs; forbid direct edits under ~/.config/bashy; bump .sibling-pins
seq: 261
status: done
priority: p1
created: 2026-09-12T23:03:30.212081Z
weave: 6
assignee: qiangli
sprint: 161
closed: 2026-09-13T00:49:12.158425Z
resolution: fixed
closed_by: claude-f
---

Taught surface + e2e for the CRUD verbs (coreutils C2/C3/C4).
- skills/bashy/SKILL.md and the BASHY_AGENT_MANIFEST text (internal/agentos/agentos.go:197) teach: never edit files under ~/.config/bashy/; use <noun> set --set, <noun> schema, skill add/set/rm/show --yaml.
- docs/command-atlas.md rows for the new subcommands.
- internal/agentos/commands_e2e_test.go: skill show conductor --yaml; skill rm of a just-added local skill; tool set --set round trip — all against scratch stores (BASHY_FLEET_DIR / BASHY_SKILLS_DIR set in the test; NEVER the operator's real store — a past test wrote the real sprint store).
- Bump .sibling-pins coreutils in the same commit (pre-push hook refuses drift).
Gate: go test ./internal/agentos/...; go test -tags e2e -run TestE2EAllListedCommandsDispatch ./internal/agentos; CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/bashy.
