---
id: 67cd371d28c0
kind: feature
title: Expose Bashy commands as MCP tool calls
seq: 346
status: done
labels:
    - mcp
created: 2026-09-30T06:59:31.168879Z
assignee: claude-fable5.1
sprint: 336
sprint_id: bfaa5508-ced5-5ec0-b4b3-56450291510c
sprint_title: Bashy built-in MCP server
closed: 2026-10-06T07:07:19.768983Z
closed_by: claude-fable5.1
---

Build a Bashy-owned MCP server exposing supported current-OS commands, including commands added through bashy commands CRUD. Preserve Bashy command semantics. Preferences recorded so far: direct per-command tools plus compact list_tools/run_tool; stdio and loopback HTTP; independent calls and persistent interactive shell sessions. SSH-launched stdio is the remote path. Server scope only; consuming external MCP servers is separate work. Refine acceptance and add further stories before implementation.

Operator correction 2026-10-01: command documentation is agent-facing. The pure-Go `man` utility being developed in Sprint 341 emits original command guidance as an MCP-shaped tool descriptor (`name`, `description`, `inputSchema`) with a `tools/call` example. Reuse that documentation model as the source for this server's direct per-command tools, and validate the wire form with the official `github.com/modelcontextprotocol/go-sdk` rather than creating a separate prose manual/schema registry. Keep POSIX `man -k` keyword search behavior.
