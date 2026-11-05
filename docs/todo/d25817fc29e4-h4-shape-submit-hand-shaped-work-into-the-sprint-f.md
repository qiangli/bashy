---
id: d25817fc29e4
kind: feature
title: 'H4 shape submit: hand shaped work into the sprint flow'
seq: 3074
status: todo
priority: p2
labels:
    - clean-baseline
    - shaping
created: 2026-11-05T01:04:41.955603Z
sprint: 564
sprint_id: 0f09b646-9acd-5bec-849d-2cf3b0e4edef
sprint_title: 'Software shaping for Bashy apps: any user shapes an app on their own host'
---

Depends on H2. bashy shape submit <app> <rev> [--story ID], plus a matching POST /api/shape/{app}/submit so the web page can do the same: upstream an approved version through bashy src pr open into a weave run linked to the candidate's tracked item (auto-created todo or the linked story), record the shaped identity on it, and go through the existing review/merge gates only (no new mandatory gate). Submitted commits carry Shape: <app>@<version> and Shaped-By: <actor>[ for <represented user>] beside the existing Sprint/Story/Story-ID trailers; the commit-msg hook checks they match the linked item. Planning a submit into a sprint is optional. Sharing stays offer/adopt; Sprint 570 handoff offers can hand the submitted work to a reviewer or manager. Acceptance: a submitted shape is reviewable and mergeable like any weave run; nothing merges without the normal gates; CLI and web submit produce identical records and trailers.

Plan: docs/plan-s564-master-execution.md. Planning record; no implementation started. One shaping model, two front-ends: the existing web /shape/<app>/ page and /api/shape/{app}, and a new bashy shape CLI mirroring that API verb for verb; both use the shared yoke/pkg/shape engine; bashy src stays the version/source plumbing underneath. Same verbs and authorization for humans, agents and agent-assisted humans. Candidates always built in an isolated workspace, never a shared checkout. Sprint 526 web behavior unchanged. Tracking always (auto todo, label shaping, origin: shape); every other ceremony optional, decided case by case by the human or agent. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

Hello World tutorial/fixture: [walkthrough](../../examples/shaping/hello-world/README.md). Reuse pinned source in isolated runs; this documentation is not E2E pass evidence.
