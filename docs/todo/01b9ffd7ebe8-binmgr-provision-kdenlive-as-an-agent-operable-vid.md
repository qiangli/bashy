---
id: 01b9ffd7ebe8
kind: feature
title: Binmgr-provision Kdenlive as an agent-operable video editor
seq: 347
status: todo
labels:
    - mcp
    - video
created: 2026-09-30T07:05:03.872971Z
sprint: 336
sprint_id: bfaa5508-ced5-5ec0-b4b3-56450291510c
sprint_title: Bashy built-in MCP server
---

Make Kdenlive available as a binmgr-provisioned Bashy command and expose agent video-editing operations through the Bashy command/MCP surface. References: https://github.com/KDE/kdenlive (upstream editor); https://github.com/D-Ogi/kdenlive-api (Python D-Bus scripting API); https://github.com/D-Ogi/mcp-kdenlive (Python MCP tool surface). The reference API/server require a patched Kdenlive build with D-Bus scripting support, so establish a provisionable compatible editor build rather than assuming stock Kdenlive exposes that API. Port the useful automation to Bash# or rewrite it in Go; choose after feasibility and licensing review. Keep the editor as a managed external binary, not linked into Bashy. Refine platform coverage, tool scope, and acceptance before implementation.
