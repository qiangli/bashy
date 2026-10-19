---
id: ee69f18dbb08
kind: feature
title: 'S11 Codync joins by declaration: examples/fleet/tools/codync.yaml with codync:* commands'
seq: 1873
status: done
priority: p2
labels:
    - bashy
created: 2026-10-11T03:52:05.810281Z
assignee: codex-gpt6-sol
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
closed: 2026-10-11T13:41:25.436772Z
closed_by: codex-gpt6-sol
---

Custom tool yaml for codync-host (Apache-2.0, download+exec, never vendored): commands codync:bots, codync:send BOT TEXT (effects agent-run, --yes), codync:approve ID allow|deny, codync:usage over its loopback API (POST /api/METHOD, bearer token from the codync token file); declare codync-host mcp team as an MCP server. Header documents the reverse direction: a Codync bot with command bashy acp drives any bashy agent. Availability probe checks the running host, not just the binary. Acceptance: tool cmd list shows codync:*; codync:send reaches a running host; file stamped verified only after a live run.

Manager acceptance 2026-10-11 13:40 UTC: source `56b42a30` plus the manager's `agent-run` effect annotation declares four generic HTTP commands, loopback bearer token, typed botId/text, allow→allow_once and deny→reject_once, and explicit mutating admission; it contains no Codync-specific runner branch. Independent Mac `GOWORK=off go test -short -count=1 -run '^TestCodync' ./internal/agentos` PASS on the final YAML. On claimed DO Linux with exact Bashy binary built from `22d3a3ec`, the manager imported the final YAML into an isolated BASHY_HOME, placed the installed Codync v2.13.2 host on a private PATH, and checked all four command availability probes. Real `codync:bots` and `codync:usage` returned HTTP 200. `codync:send --yes -- BOT TEXT` delivered the exact botId and text to the existing Sprint418 test bot with HTTP 200, including a repeat after the final annotation; the same command without `--yes` exited 1 before HTTP. The local fake-loopback regression also checks `codync:approve` allow/deny projections, token and request body. The YAML is now stamped with this dated live verification. S10 reverse ACP and S20 Codync route remain separate original outcomes.

Sprint 418 managed assignment: own only `examples/fleet/tools/codync.yaml`, adjacent example documentation, and focused declaration tests. Do not edit Bashy S16 monitor, S4 confirmation, S19 scratch runner, yoke S10 ACP or outpost S18 source. Pin any downloaded artifact with a real release/digest and use the existing generic tool declaration machinery; keep a `verified` stamp absent until the manager's live Codync host gate actually passes. Worker gate: parse/import/show/list the YAML in an isolated registry, exercise each declared command against a local fake loopback API with bearer token and exact effects, plus Bashy focused Go tests for the declaration path. The manager owns the real live host, reverse ACP and remote native gates. Commit with Sprint #418, this Story-ID and Agent trailer. dev-host short tests only.
