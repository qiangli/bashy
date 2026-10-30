---
id: d2e7348ee67a
kind: bug
title: 'Windows: dag effect classifier treats ''tool.exe'' as unknown (needs unknown not in Effects)'
seq: 2967
status: todo
priority: p1
labels:
    - windows
    - dag
created: 2026-10-30T21:13:25.45908Z
---

Found by Sprint 537 W5 on noviwin1 (Windows 11 amd64, bashy built from main 36b1e125), 2026-10-30. A dag task whose body runs a Go-built tool by its Windows name fails the effect cap:
  dag: effect cap: target "roundtrip": "form.exe" needs unknown not in Effects: cred,destroy,exec,net,read,write
The same recipe passes on macOS/Linux where the binary is "form". Expected: the effect classifier resolves the command the same way on every OS (strip PATHEXT suffixes such as .exe/.cmd/.bat before lookup, or classify by resolved path), so a recipe needs no per-OS names. Red/green: classifier unit test with "tool.exe" vs "tool" yielding the same effects on Windows (or via a seam).
