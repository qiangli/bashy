---
id: d11cb989dfbf
kind: bug
title: Preserve case-insensitive Windows GNU Bash helper argv0 dispatch
seq: 331
status: todo
priority: p0
labels:
    - windows
    - release
created: 2026-09-23T20:32:23.184429Z
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

Bashy test workflow 35915920628 at 62ad095 exposes tools/bash53suite/TestRunAsHelperDispatchesByArgv0 failure on Windows after Story #330 passes. runAsHelper trims lowercase .exe before Windows case folding, so RECHO.EXE remains recho.exe and misses dispatch. Repair only the Windows suffix/name normalization without changing Unix helper dispatch or GNU Bash fixture semantics; require native focused Windows control, Unix control/cross-build, and green three-OS test CI before v0.28.0-dev.
