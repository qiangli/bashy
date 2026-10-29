---
id: 99462b87ea6b
kind: bug
title: 'bashy on Windows: system32 tools (wsl.exe, powershell.exe) not found from bashy shell'
seq: 2742
status: todo
priority: p2
labels:
    - windows
created: 2026-10-27T17:56:42.973008Z
---

Spotted by s517-manager probing a Windows host 2026-10-27: inside bashy, 'wsl.exe' and 'powershell.exe' -> command not found, though C:\windows\system32\wsl.exe exists; 'which bashy' resolves but system32 appears absent from the lookup PATH in the outpost bashy session. Reproduce, fix PATH/tool resolution so host system tools resolve (or document the full-path rule). Not blocking Sprint 517 (bashy podman/go provision); may matter for WSL/pwsh lanes. Per operator rule: bashy is all you need - a missing capability gets a story.
