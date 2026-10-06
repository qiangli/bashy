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
sprint: 382
sprint_id: 52e1124d-d9d2-5528-bbfa-80f6b8538c1d
sprint_title: Agent-operable open-source creative tools through binmgr and registered commands
---

Provision D-Ogi/kdenlive (https://github.com/D-Ogi/kdenlive), the modified Kdenlive fork with the D-Bus scripting API required by https://github.com/D-Ogi/kdenlive-api and https://github.com/D-Ogi/mcp-kdenlive. KDE/kdenlive (https://github.com/KDE/kdenlive) is upstream reference, not the provisioning target. Make the compatible editor available as a binmgr-managed Bashy command and expose agent video-editing operations through the Bashy command/MCP surface. Port the useful Python automation to Bash# or rewrite it in Go; choose after feasibility and licensing review. Keep the editor as a managed external binary, not linked into Bashy. Refine packaging, platform coverage, tool scope, and acceptance before implementation.
