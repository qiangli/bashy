---
id: da9817fe0495
kind: bug
title: Restore Bashy three-OS test workflow before Sprint 341 certification freeze
seq: 381
status: done
priority: p0
created: 2026-10-01T05:32:09.628247Z
assignee: codex-gpt6.1-sol
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
closed: 2026-10-02T09:48:34.707928Z
closed_by: codex-gpt6.1-sol
---

Runbook 3 bashy-release requires the candidate test workflow green on Linux, macOS, Windows. Current 57496b9 run 36812832493 fails remote-install fixture discovery on all OS; Windows also fails peer channel tests. Build the intended test binary and fix genuine platform regressions. Acceptance: local build and tests pass, pushed exact candidate has three green jobs, then freeze certification pins.
