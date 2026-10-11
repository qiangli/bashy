---
id: abc4ce65c606
kind: feature
title: 'S10 ACP everywhere: claude/codex/gemini acp_exec, updates as events, bashy acp with permission relay'
seq: 1872
status: todo
priority: p2
labels:
    - yoke
    - bashy
created: 2026-10-11T03:52:03.478684Z
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
---

Add acp_exec to yoke/pkg/fleet/baseline/tools claude, codex and gemini via pinned ACP adapters (download records, sha256); map tool_call, plan, thought and usage updates into S3 entry events instead of dropping them (yoke/pkg/chat/acp.go); bashy acp = herald acp, relaying request_permission from S4; publish an ACP-registry manifest. Acceptance: Zed and a Codync bot whose command is bashy acp each complete a prompt; a permission request reaches bashy approve. Refs docs/herald-agent-interop-design.md.

Manager continuation 2026-10-11 11:56 UTC: moved the same Story-ID from umbrella to Bashy for a pointed, private worker. Current Bashy `244740fe` runs `bashy herald acp` but has no top-level `bashy acp`; Yoke already wires `acp.ApprovalHandler` through `pkg/chat/acp.go`. Implement only the Bashy top-level dispatch/help parity and one process-level ACP initialize/session/prompt test in `internal/agentos/agentos.go`, `commands.go` and focused tests. Verify with `GOWORK=off go test -short -timeout 5m ./internal/agentos/...` plus a built `bashy acp --help` and fake-runner JSON-RPC exchange. Do not duplicate Yoke's existing permission relay or add a framework. Submit a bounded partial receipt; original S10 remains open for the pinned adapters/update events, real Zed and Codync bot prompts and an S4 permission request reaching `bashy approve`. Do not touch active Yoke S6 meet or S13 task-feed/ask files.
