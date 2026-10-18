---
id: c47a9f514504
kind: feature
title: Binmgr-provision Natron for agent-operable VFX compositing
seq: 352
status: todo
priority: p2
created: 2026-09-30T07:29:25.109843Z
sprint: 468
sprint_id: f49fe122-2ad4-502a-a597-b16faf478de3
sprint_title: Licensing- and artifact-blocked tools
---

Provision open-source Natron (GPLv2; https://github.com/NatronGitHub/Natron) as a binmgr-managed external command for film-shot compositing. Expose agent-usable project, node graph, render, and output inspection operations through Bashy command/MCP tools. Evaluate Natron Python scripting and NatronRenderer/headless CLI as integration seams. Use an existing MCP bridge only if its license is permissive and compatibility is verified; otherwise implement a Go bridge over the supported scripting/CLI surface. Refine platform coverage and operation scope before implementation.
