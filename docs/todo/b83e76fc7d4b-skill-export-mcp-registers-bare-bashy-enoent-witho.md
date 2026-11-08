---
id: b83e76fc7d4b
kind: task
title: skill export --mcp registers bare 'bashy', ENOENT without PATH
seq: 3188
status: todo
created: 2026-11-08T01:46:28.473925Z
sprint: 591
sprint_id: dc240e08-28b0-5592-85a1-15e3d650c716
sprint_title: 'README as agent bootstrap: human hook + agent skill, one consent gate'
---

claude mcp add --scope user bashy -- bashy mcp serve: a new session whose PATH lacks ~/.local/bin gets 'ENOENT: Executable not found in $PATH: bashy'. Register the absolute path of the running executable. Cold-start container, 2026-11-07.
