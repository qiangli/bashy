---
id: 67cd371d28c0
kind: feature
title: Expose Bashy commands as MCP tool calls
seq: 346
status: todo
labels:
    - mcp
created: 2026-09-30T06:59:31.168879Z
sprint: 336
sprint_id: bfaa5508-ced5-5ec0-b4b3-56450291510c
sprint_title: Bashy built-in MCP server
---

Build a Bashy-owned MCP server exposing supported current-OS commands, including commands added through bashy commands CRUD. Preserve Bashy command semantics. Preferences recorded so far: direct per-command tools plus compact list_tools/run_tool; stdio and loopback HTTP; independent calls and persistent interactive shell sessions. SSH-launched stdio is the remote path. Server scope only; consuming external MCP servers is separate work. Refine acceptance and add further stories before implementation.
