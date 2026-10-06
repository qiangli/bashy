---
id: c47a9f514504
kind: feature
title: Binmgr-provision Natron for agent-operable VFX compositing
seq: 352
status: todo
created: 2026-09-30T07:29:25.109843Z
sprint: 382
sprint_id: 52e1124d-d9d2-5528-bbfa-80f6b8538c1d
sprint_title: Agent-operable open-source creative tools through binmgr and registered commands
---

Provision open-source Natron (GPLv2; https://github.com/NatronGitHub/Natron) as a binmgr-managed external command for film-shot compositing. Expose agent-usable project, node graph, render, and output inspection operations through Bashy command/MCP tools. Evaluate Natron Python scripting and NatronRenderer/headless CLI as integration seams. Use an existing MCP bridge only if its license is permissive and compatibility is verified; otherwise implement a Go bridge over the supported scripting/CLI surface. Refine platform coverage and operation scope before implementation.
