---
id: f5817629a95b
kind: feature
title: Binmgr-provision Blender for agent-operable 3D and VFX
seq: 353
status: done
priority: p1
created: 2026-09-30T07:29:25.193415Z
weave: 214
assignee: claude-fable5.1
sprint: 382
sprint_id: 52e1124d-d9d2-5528-bbfa-80f6b8538c1d
sprint_title: Agent-operable open-source creative tools through binmgr and registered commands
closed: 2026-10-18T07:38:54.75406Z
closed_by: s382-manager
---

Provision open-source Blender (GPL; https://github.com/blender/blender) as a binmgr-managed external command for film previs, animation, tracking, and 3D/VFX. Expose agent-usable scene, asset, render, and output inspection operations through Bashy command/MCP tools. Use Blender background mode and Python API as the primary automation seam; an MCP bridge may be adopted only with a verified permissive license, otherwise implement the bridge in Go. Keep Blender external to Bashy and refine platform coverage and operation scope before implementation.
