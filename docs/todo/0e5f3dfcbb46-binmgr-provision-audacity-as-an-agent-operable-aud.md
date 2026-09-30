---
id: 0e5f3dfcbb46
kind: feature
title: Binmgr-provision Audacity as an agent-operable audio editor
seq: 349
status: todo
labels:
    - mcp
    - audio
created: 2026-09-30T07:12:21.616026Z
sprint: 336
sprint_id: bfaa5508-ced5-5ec0-b4b3-56450291510c
sprint_title: Bashy built-in MCP server
---

Select open-source Audacity (https://github.com/audacity/audacity) as the audio editor and provision a compatible Audacity 3.x release as a binmgr-managed Bashy command. Expose useful audio-editing operations through the Bashy command/MCP surface, using https://github.com/xDarkzx/Audacity-MCP (Apache-2.0) and https://manual.audacityteam.org/man/scripting.html as references. The reference server drives Audacity via mod-script-pipe, which must be enabled and currently targets Audacity 3.x; Audacity 4 compatibility requires separate evaluation. Keep the scripting pipe local to the same user and do not expose it as a network service. Adapt the useful automation to Bash# or Go where feasible; keep Audacity as a managed external binary, not linked into Bashy. Refine packaging, platform coverage, tool scope, and acceptance before implementation.
