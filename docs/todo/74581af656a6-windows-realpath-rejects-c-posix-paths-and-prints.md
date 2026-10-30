---
id: 74581af656a6
kind: bug
title: 'Windows: realpath rejects /c/... POSIX paths (and prints C:\ form for ''.'')'
seq: 2965
status: todo
priority: p1
labels:
    - windows
    - coreutils
created: 2026-10-30T20:57:53.81752Z
---

Found by Sprint 537 W5 on noviwin1 (Windows 11 amd64) with bashy built from main d929f8b3, 2026-10-30. bashy's realpath does not accept the POSIX drive form that the rest of the shell uses:
  d=$HOME/s537/mvt; mkdir -p "$d"; realpath "$d"    # -> realpath: /c/Users/noviadmin/s537/mvt: No such file or directory
  realpath .                                        # -> C:\Users\noviadmin\s537\mvt\a  (Windows form)
  realpath --relative-to="$here" "$bin"             # fails the same way
cd, mv, cp, mkdir, ls accept /c/Users/... in the same session. Expected: realpath resolves /c/... like every other path-taking builtin and prints the same path form the shell uses ($PWD style), identical on every OS. Coreutils is certified; this is a bug fix (Windows path translation in realpath), red/green test with a /c/ path on Windows (or the seam the other utils use).
