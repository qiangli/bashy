---
id: 0556ec75d152
kind: feature
title: Binmgr-provision Ardour as an agent-operable music editor
seq: 350
status: todo
labels:
    - mcp
    - music
created: 2026-09-30T07:16:22.346919Z
sprint: 336
sprint_id: bfaa5508-ced5-5ec0-b4b3-56450291510c
sprint_title: Bashy built-in MCP server
---

Select open-source Ardour (GPLv2; https://git.ardour.org/ardour/ardour) for music production and editing: multitrack recording, MIDI, plugins, mixing, and export. Provision a compatible Ardour build as a binmgr-managed Bashy command, then expose useful project/track/transport/edit/mix operations through the Bashy command/MCP surface. Ardour documents OSC control (https://manual.ardour.org/using-control-surfaces/controlling-ardour-with-osc/osc-control/) and Lua scripting (https://manual.ardour.org/lua-scripting/); https://github.com/pyroqbit/ardour-mcp is an early MIT-licensed OSC-to-MCP reference, not a required dependency. Keep control local to the user and choose a reproducible open-source build/artifact before committing binmgr pins; official ready-to-run binaries may require payment, while source is available without charge. Adapt useful automation to Bash# or Go where feasible. Keep Ardour as a managed external binary, not linked into Bashy. Refine platform coverage, tool scope, and acceptance before implementation. Audacity remains the waveform/audio-cleanup story.
