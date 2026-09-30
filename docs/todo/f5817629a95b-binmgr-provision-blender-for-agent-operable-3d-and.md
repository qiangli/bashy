---
id: f5817629a95b
kind: feature
title: Binmgr-provision Blender for agent-operable 3D and VFX
seq: 353
status: todo
created: 2026-09-30T07:29:25.193415Z
sprint: 336
sprint_id: bfaa5508-ced5-5ec0-b4b3-56450291510c
sprint_title: Bashy built-in MCP server
---

Provision open-source Blender (GPL; https://github.com/blender/blender) as a binmgr-managed external command for film previs, animation, tracking, and 3D/VFX. Expose agent-usable scene, asset, render, and output inspection operations through Bashy command/MCP tools. Use Blender background mode and Python API as the primary automation seam; an MCP bridge may be adopted only with a verified permissive license, otherwise implement the bridge in Go. Keep Blender external to Bashy and refine platform coverage and operation scope before implementation.
