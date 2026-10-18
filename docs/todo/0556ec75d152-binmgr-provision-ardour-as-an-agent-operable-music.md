---
id: 0556ec75d152
kind: feature
title: Binmgr-provision Ardour as an agent-operable music editor
seq: 350
status: todo
priority: p2
labels:
    - mcp
    - music
created: 2026-09-30T07:16:22.346919Z
sprint: 468
sprint_id: f49fe122-2ad4-502a-a597-b16faf478de3
sprint_title: Licensing- and artifact-blocked tools
---

Select open-source Ardour (GPLv2; https://git.ardour.org/ardour/ardour) for music production and editing: multitrack recording, MIDI, plugins, mixing, and export. Provision a compatible Ardour build as a binmgr-managed Bashy command, then expose useful project/track/transport/edit/mix operations through the Bashy command/MCP surface. Ardour documents OSC control (https://manual.ardour.org/using-control-surfaces/controlling-ardour-with-osc/osc-control/) and Lua scripting (https://manual.ardour.org/lua-scripting/); https://github.com/pyroqbit/ardour-mcp is an early MIT-licensed OSC-to-MCP reference, not a required dependency. Keep control local to the user and choose a reproducible open-source build/artifact before committing binmgr pins; official ready-to-run binaries may require payment, while source is available without charge. Adapt useful automation to Bash# or Go where feasible. Keep Ardour as a managed external binary, not linked into Bashy. Refine platform coverage, tool scope, and acceptance before implementation. Audacity remains the waveform/audio-cleanup story.
