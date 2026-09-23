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

## Focused repair, 2026-09-23

On Windows, fold the basename to lowercase before trimming the `.exe` suffix;
Unix still trims only literal lowercase `.exe` and keeps case-sensitive helper
selection. The existing `TestRunAsHelperDispatchesByArgv0` covers uppercase
`RECHO.EXE` and the harness non-dispatch controls. Focused macOS `go test
./tools/bash53suite -run '^TestRunAsHelperDispatchesByArgv0$' -count=1` passed,
and Windows amd64 test-executable cross-compilation passed (binary SHA-256
`78ce970beb17f073be4163ef73ced7d60c978d39808aac90ee5d778a546676ed`).
On noviwin1 that exact executable ran the focused test natively and passed;
retained log `/tmp/s250-331-windows-focused.log` SHA-256
`cbae22c15364083755af13a7abcfd136cad3312b58c5b4841416e5efc83e3193`.
Three-OS test CI remains the release acceptance gate before `v0.28.0-dev`.
