---
id: 1bae7aa7aebd
kind: bug
title: S557 repair Desk full-suite timeout without weakening behavioral coverage
seq: 3041
status: assigned
priority: p1
created: 2026-11-01T19:27:07.086888Z
weave: 2
assignee: s557-manager
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
---

Sprint557 migration repair. Work only in assigned isolated workspace. Read local CLAUDE/AGENTS. Preserve all test assertions, coverage, thresholds and deadlines; diagnose before modifying. No release/push/parent pin changes. No heavy/full local builds/suites: bounded targeted tests only GOMAXPROCS=2 GOFLAGS=-p=2, full validation hosted by manager. Commit named files with Sprint: #557 and correct Story/Story-ID trailers and injected BASHY_AGENT. Report exact evidence and limitations. Full hosted run41332672873 atBashy86021a96 failed only examples/desk timeout10m; TestDeskToolsForkAndRollback chart/form waiting child CombinedOutput. Log /tmp/s557-bashy-full-41332672873-failed.log. Inspect nested go-run build/runtime phases; eliminate redundant build work or actual leak without increasing behavior deadlines. Keep all tool fork/rollback scenarios.
