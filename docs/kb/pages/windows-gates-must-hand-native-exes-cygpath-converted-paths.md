---
id: 01a136d8-1431-72b7-91ca-b0f6f6ab4af1
seq: 5
form: page
type: gotcha
title: Windows gates must hand native exes cygpath-converted paths
description: 'When a Git Bash gate script invokes a native Windows executable (python.exe, cmake.exe, a compiled WFP guard): convert every path that binary opens, filters, or spawns with cygpath -m. MSYS auto-converts slash paths heuristically, but app-id filtering, CreateProcess, and file opens fail closed on POSIX /c/... forms, so pass explicit Windows paths for pidfiles, executables, models, and CMake sources.'
status: candidate
source:
    tool: muse-spark1.3-w106-b
    episode: weave-issue-106
created: "2026-10-13T19:58:13Z"
---
