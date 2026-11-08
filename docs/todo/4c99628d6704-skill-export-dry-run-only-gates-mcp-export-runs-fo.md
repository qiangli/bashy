---
id: 4c99628d6704
kind: task
title: skill export --dry-run only gates MCP; export runs for real
seq: 3187
status: todo
created: 2026-11-08T01:46:28.309659Z
sprint: 591
sprint_id: dc240e08-28b0-5592-85a1-15e3d650c716
sprint_title: 'README as agent bootstrap: human hook + agent skill, one consent gate'
---

bashy skill export bashy --user --tool claude --mcp --dry-run prints the mcp add command but writes every skill folder and instruction block. Expected: --dry-run previews the whole command (or is refused when --mcp is absent). Reproduced twice on 2026-11-07 (operator dev box and cold-start container).
