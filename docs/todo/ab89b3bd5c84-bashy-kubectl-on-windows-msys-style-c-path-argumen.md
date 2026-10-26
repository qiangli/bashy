---
id: ab89b3bd5c84
kind: bug
title: 'bashy kubectl on Windows: MSYS-style /c/... path arguments (e.g. --kubeconfig /c/Users/.../tmp.X from mktemp) are not converted for the native kubectl, which then falls back to localhost:8080'
seq: 2724
status: todo
priority: p0
created: 2026-10-24T20:11:55.678392Z
sprint: 507
sprint_id: 77d7ab0f-6aef-5bd3-876d-3f50e9b585ea
sprint_title: 'YC demo blockers: four bug fixes (dks reattach, executor image default, AMD VRAM on Windows, kubectl /c/ paths)'
---

Found 2026-10-24 running examples/dks/join.bsh on elephant (Windows, bashy rc.12): kc=$(mktemp) gives /c/Users/liqia/AppData/Local/Temp/tmp.X; 'bashy kubectl --kubeconfig "$kc" get nodes' -> kubectl tries http://localhost:8080 (file not read). kubectl is binmgr-managed, so per the Windows argv-conversion rule its path arguments should be converted. Also 'winpath' is not available on that host. Workaround in join.bsh: native C:/ path under LOCALAPPDATA.
