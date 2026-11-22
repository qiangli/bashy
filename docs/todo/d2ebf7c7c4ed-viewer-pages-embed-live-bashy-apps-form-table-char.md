---
id: d2ebf7c7c4ed
kind: feature
title: viewer pages embed live bashy apps (form, table, chart, notepad, ...) so a plan and its decision form, data or chart live on one page
seq: 3281
status: todo
priority: p2
created: 2026-11-21T09:08:25.660539Z
---

Operator request 2026-11-21. Context: the conductor showed a sprint plan in bashy viewer (book sprint-677) and its four decisions as a separate bashy form (form-f1bc68d375d5); the operator wants them on one page. Shape: a viewer page can reference another app item (form/table/chart/notepad/...) by id and render it live in place, keeping that app's own sharing, owner key and invite contract (an embed must not widen access; answers still go to the owning app's store). Generic across desk-kit apps, not form-specific. Build on the desk kit renderer (S537.N3 d1c93073, file-type embeds) and the shaped-app runtime discovery. Open questions: same-origin vs iframe per app port, CSP, how an embedded item's invite is redeemed. Acceptance: one viewer page showing a plan page plus an embedded live form whose Submit lands in the form store, verified in a real browser on the gate host (never Chrome tests on the dev box).
