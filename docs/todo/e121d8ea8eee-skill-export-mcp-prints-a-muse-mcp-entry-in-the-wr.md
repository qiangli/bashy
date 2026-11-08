---
id: e121d8ea8eee
kind: task
title: skill export --mcp prints a muse MCP entry in the wrong shape
seq: 3193
status: todo
created: 2026-11-08T02:32:02.976854Z
sprint: 591
sprint_id: dc240e08-28b0-5592-85a1-15e3d650c716
sprint_title: 'README as agent bootstrap: human hook + agent skill, one consent gate'
---

For --tool muse, bashy cannot run a CLI registration and prints an entry to add; muse rejected that shape until the agent rewrote it with schema_version: 1, type: "stdio", mode: "optional" in ~/.config/muse/settings.json. Emit muse's current settings shape (or write it via muse's own command if one exists). Cold start 2026-11-08, muse 1.4.3.
