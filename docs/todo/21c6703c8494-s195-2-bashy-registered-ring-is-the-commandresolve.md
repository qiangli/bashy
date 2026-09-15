---
id: 21c6703c8494
kind: task
title: 'S195.2 bashy: registered ring is the CommandResolver; type/command -v see a registered name'
seq: 298
status: todo
priority: p0
created: 2026-09-15T15:54:00.75167Z
sprint: 195
---

Wire interp.CommandResolver(registeredResolver) on BOTH wireExec branches (posix + agentic). registeredLookup already stands down under VSC_PROFILE=cert. Before: bashy -c hi runs (rc 0) while type hi / command -v hi say not found (rc 1) — a script probing with command -v before calling a registered command wrongly concludes it is absent.
