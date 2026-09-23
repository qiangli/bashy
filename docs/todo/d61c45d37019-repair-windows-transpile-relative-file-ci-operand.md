---
id: d61c45d37019
kind: bug
title: Repair Windows transpile relative-file CI operand
seq: 330
status: todo
priority: p0
labels:
    - windows
    - release
created: 2026-09-23T20:10:53.31444Z
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

Bashy test workflow run 35908988886 on candidate e48bd7d fails TestTranspileModuleInputDirectory/relative-file: transpile cannot open ..\module\main.bpp, shown with U+F05C encoded backslashes. The Go test uses filepath.Rel, which emits native backslashes on Windows; the transpile CLI path boundary currently treats relative backslashes as literal POSIX filename characters. Determine the intended native-vs-shell relative operand contract, then make the smallest sound product or test correction with controls for actual Windows dispatch and literal backslashes. Keep existing source/fixture semantics and limits. Gate: focused Windows runtime test, macOS/Linux tests, and Bashy three-OS test CI green before release tagging.
