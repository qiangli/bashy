---
id: 9505bb10b74a
kind: feature
title: Binmgr-provision Story Architect for agent-operable screenwriting
seq: 351
status: todo
created: 2026-09-30T07:29:25.018426Z
sprint: 336
sprint_id: bfaa5508-ced5-5ec0-b4b3-56450291510c
sprint_title: Bashy built-in MCP server
---

Provision open-source Story Architect (GPLv3; https://github.com/story-apps/starc) as a binmgr-managed external command for screenplay and drama-series writing; package its open-source components without requiring commercial modules. Expose agent-usable project, script, scene, character, and series operations through Bashy command/MCP tools. Assess https://github.com/Christianrhf/starc-mcp as a format and workflow reference only: the repository has no visible license, so do not copy or depend on its implementation without a permissive license. Prefer a permissively licensed MCP bridge; otherwise write the integration in Go against documented or validated file/application interfaces. Validate compatibility and safe project writes before delivery. Refine platform coverage and operation scope before implementation.
