---
id: 8c9debe56c0b
kind: refactor
title: S575 move shell CLI ownership and change product package references only
seq: 3244
status: assigned
priority: p0
created: 2026-11-18T04:11:45.552735Z
assignee: s575-codex-manager
sprint: 575
sprint_id: 65ce85f5-87c0-5863-bf55-62de86f5df36
sprint_title: 'Layer separation: Classic <- Bash# <- Yoke import-graph delineation'
---

Mechanical source separation first. Relocate internal/cli unchanged into bashsharp/cli; point product imports to it and unchanged whole Bash engine. Preserve all implementation and tests, except relocated source ownership and package references. Update fixed published dependency pins. Defer mixed sh refactoring. Build ./... with GOWORK=off in locked official image.
