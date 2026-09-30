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

Provision D-Ogi/kdenlive (https://github.com/D-Ogi/kdenlive), the modified Kdenlive fork with the D-Bus scripting API required by https://github.com/D-Ogi/kdenlive-api and https://github.com/D-Ogi/mcp-kdenlive. KDE/kdenlive (https://github.com/KDE/kdenlive) is upstream reference, not the provisioning target. Make the compatible editor available as a binmgr-managed Bashy command and expose agent video-editing operations through the Bashy command/MCP surface. Port the useful Python automation to Bash# or rewrite it in Go; choose after feasibility and licensing review. Keep the editor as a managed external binary, not linked into Bashy. Refine packaging, platform coverage, tool scope, and acceptance before implementation.
