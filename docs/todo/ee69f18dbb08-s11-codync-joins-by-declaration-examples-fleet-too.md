---
id: ee69f18dbb08
kind: feature
title: 'S11 Codync joins by declaration: examples/fleet/tools/codync.yaml with codync:* commands'
seq: 1873
status: assigned
priority: p2
labels:
    - bashy
created: 2026-10-11T03:52:05.810281Z
weave: 26
assignee: codex-gpt5.6-sol
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
---

Custom tool yaml for codync-host (Apache-2.0, download+exec, never vendored): commands codync:bots, codync:send BOT TEXT (effects agent-run, --yes), codync:approve ID allow|deny, codync:usage over its loopback API (POST /api/METHOD, bearer token from the codync token file); declare codync-host mcp team as an MCP server. Header documents the reverse direction: a Codync bot with command bashy acp drives any bashy agent. Availability probe checks the running host, not just the binary. Acceptance: tool cmd list shows codync:*; codync:send reaches a running host; file stamped verified only after a live run.

Sprint 418 managed assignment: own only `examples/fleet/tools/codync.yaml`, adjacent example documentation, and focused declaration tests. Do not edit Bashy S16 monitor, S4 confirmation, S19 scratch runner, yoke S10 ACP or outpost S18 source. Pin any downloaded artifact with a real release/digest and use the existing generic tool declaration machinery; keep a `verified` stamp absent until the manager's live Codync host gate actually passes. Worker gate: parse/import/show/list the YAML in an isolated registry, exercise each declared command against a local fake loopback API with bearer token and exact effects, plus Bashy focused Go tests for the declaration path. The manager owns the real live host, reverse ACP and remote native gates. Commit with Sprint #418, this Story-ID and Agent trailer. Dragon short tests only.
