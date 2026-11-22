---
id: 31d1e09e350e
kind: feature
title: C1 viewer and chart become shaped apps (Genie button, CLI follows console address)
seq: 3258
status: done
priority: p1
created: 2026-11-20T19:14:04.014111Z
assignee: codex-gpt6.2-sol
sprint: 682
sprint_id: 623586bb-742f-570e-87c9-9af0c101bac1
sprint_title: Shaping-enable the example apps for consumers (viewer, session, form, board/whiteboard/table/hello-world, notepad/chart)
closed: 2026-11-21T20:44:56.937901Z
closed_by: claude-opus5.5
---

Spec: umbrella docs/plan-s682-consumer-shaping-example-apps.md §C1. Consumer-first shaping (operator 2026-11-20); owner/producer side is Sprint 683.

## WSL verification venue (2026-11-21)

`TestAppRuntimeLifecycle` and `TestGenieOnShapedServerPage` were red when Go ran the Yoke tests from its downloaded module ZIP under Bashy `5c8be965`. Both failed at fixture setup with `lstat testdata/shaped/serverapp/app: no such file or directory`. Same pin comparison: Yoke `d5aaaef1` Git tree contains six files under that path including a nested `go.mod`; its published module ZIP contains zero. The last green C1b used a Yoke source checkout. Classification: verification venue/packaging mismatch, before any runtime assertion, with no source bisect needed. Retain this result and rerun at the same source and dependency pins with the exact Yoke checkout fixtures.
