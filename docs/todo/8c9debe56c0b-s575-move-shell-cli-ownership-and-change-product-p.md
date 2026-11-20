---
id: 8c9debe56c0b
kind: refactor
title: S575 move shell CLI ownership and change product package references only
seq: 3244
status: done
priority: p0
created: 2026-11-18T04:11:45.552735Z
assignee: s575-codex-manager
sprint: 575
sprint_id: 65ce85f5-87c0-5863-bf55-62de86f5df36
sprint_title: 'Layer separation: Classic <- Bash# <- Yoke import-graph delineation'
closed: 2026-11-20T12:26:25.488823Z
closed_by: s575-codex-manager
---

Mechanical source separation first. Relocate internal/cli unchanged into bashsharp/cli; point product imports to it and unchanged whole Bash engine. Preserve all implementation and tests, except relocated source ownership and package references. Update fixed published dependency pins. Defer mixed sh refactoring. Build ./... with GOWORK=off in locked official image.

## Manager closeout — 2026-11-20
Reference-only product cutover published at 9a6d5824, with retained official product compilation. Shared CLI ownership moved to Bashsharp and whole engine stayed in Bash. This accepts relocation/import ownership only; deferred interpreter separation is not claimed.
Existing receipts and source ancestry were reviewed; no new test/build ran during reconciliation. Evidence belongs to its exact historical source/artifact identities, not arbitrary later heads. S575 full-run evidence remains under posix-baseline/s575-2026-11-19 with its raw 4 FAIL, 54 UNRESOLVED and 9 INSPECT unchanged. RC38 is on operator hold.
