---
id: 95d4747436c8
kind: feature
title: Binmgr-provision GIMP as an agent-operable image editor
seq: 348
status: todo
labels:
    - mcp
    - image
created: 2026-09-30T07:09:10.642584Z
sprint: 336
sprint_id: bfaa5508-ced5-5ec0-b4b3-56450291510c
sprint_title: Bashy built-in MCP server
---

Provision GIMP (https://github.com/gnome/gimp) as a binmgr-managed Bashy command and expose image-editing operations through the Bashy command/MCP surface. Use https://github.com/maorcc/gimp-mcp as a reference for agent tools, live image snapshots, and the GIMP plugin bridge. The reference uses a plugin running inside GIMP plus a separate Python MCP server; account for installing/starting the compatible plugin, not just the editor executable. Adapt the useful automation to Bash# or Go where feasible, while retaining a GIMP-native plugin seam if required. Keep GIMP as a managed external binary, not linked into Bashy. Refine packaging, platform coverage, tool scope, and acceptance before implementation.
