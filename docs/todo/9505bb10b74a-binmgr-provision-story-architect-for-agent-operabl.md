---
id: 9505bb10b74a
kind: feature
title: Binmgr-provision Story Architect for agent-operable screenwriting
seq: 351
status: todo
created: 2026-09-30T07:29:25.018426Z
sprint: 382
sprint_id: 52e1124d-d9d2-5528-bbfa-80f6b8538c1d
sprint_title: Agent-operable open-source creative tools through binmgr and registered commands
---

Provision open-source Story Architect (GPLv3; https://github.com/story-apps/starc) as a binmgr-managed external command for screenplay and drama-series writing; package its open-source components without requiring commercial modules. Expose agent-usable project, script, scene, character, and series operations through Bashy command/MCP tools. Assess https://github.com/Christianrhf/starc-mcp as a format and workflow reference only: the repository has no visible license, so do not copy or depend on its implementation without a permissive license. Prefer a permissively licensed MCP bridge; otherwise write the integration in Go against documented or validated file/application interfaces. Validate compatibility and safe project writes before delivery. Refine platform coverage and operation scope before implementation.
