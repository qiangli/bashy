---
id: 6980d6206344
kind: feature
title: Binmgr-provision Kitsu for agent-operable series production tracking
seq: 354
status: todo
created: 2026-09-30T07:29:25.273713Z
sprint: 336
sprint_id: bfaa5508-ced5-5ec0-b4b3-56450291510c
sprint_title: Bashy built-in MCP server
---

Provision open-source Kitsu (AGPLv3; https://github.com/cgwire/kitsu) and required Zou API (https://github.com/cgwire/zou) as binmgr-managed external components for film/drama series production tracking. Expose agent-usable projects, episodes, sequences, shots, tasks, versions, and reviews through Bashy command/MCP tools. Evaluate MIT-licensed https://github.com/huikku/kitsu-mcp and https://github.com/INGIPSA/kitsu-mcp-server against the current Zou API; adopt a suitable permissive bridge or implement the bridge in Go. Keep credentials scoped to the user/service and require preview or dry-run support for consequential writes. Refine deployment and operation scope before implementation.
