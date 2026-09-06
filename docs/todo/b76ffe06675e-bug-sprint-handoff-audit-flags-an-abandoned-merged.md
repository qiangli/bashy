---
id: b76ffe06675e
kind: task
title: 'Bug: sprint handoff audit flags an abandoned merged weave as unclean'
seq: 242
status: todo
priority: p1
created: 2026-09-06T05:07:57.193068Z
sprint: 130
---

Found during the codex-sprint127 handoff. coreutils run #11 is state=abandoned, merged=true, workspace absent, salvageable=false, and `weave list` reports no active runs, but stale NeedsSteward/StewardReason from a failed pair launch remains. `sprint handoff` appends UNCLEAN and tells the successor to salvage/merge/abandon an already abandoned+merged run. Hygiene must ignore closed merged runs or abandonment must clear obsolete steward flags. Add a regression proving a clean handoff does not emit this false blocker.
