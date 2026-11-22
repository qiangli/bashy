---
id: 22debb3fbad2
kind: feature
title: 'C1b local discovery: bashy app show NAME --runtime so viewer/chart CLIs work without manual handoff (C1 merges with it)'
seq: 3263
status: done
priority: p1
created: 2026-11-20T20:23:02.300561Z
assignee: codex-gpt6.2-sol
sprint: 682
sprint_id: 623586bb-742f-570e-87c9-9af0c101bac1
sprint_title: Shaping-enable the example apps for consumers (viewer, session, form, board/whiteboard/table/hello-world, notepad/chart)
closed: 2026-11-21T08:38:11.931602Z
closed_by: codex-gpt6.2-sol
---

Spec: umbrella docs/plan-s682-consumer-shaping-example-apps.md §C1b. Found in conductor review of C1 (codex 189607a0): shaped registration moves the server to a console-chosen address + per-principal data dir; plain 'bashy viewer add' would hit the old fixed port. Platform change in yoke webconsole/fleet; local operator principal only.

## WSL verification venue (2026-11-21)

`TestAppRuntimeLifecycle` and `TestGenieOnShapedServerPage` were red when Go ran the Yoke tests from its downloaded module ZIP under Bashy `5c8be965`. Both failed at fixture setup with `lstat testdata/shaped/serverapp/app: no such file or directory`. Same pin comparison: Yoke `d5aaaef1` Git tree contains six files under that path including a nested `go.mod`; its published module ZIP contains zero. The last green C1b used a Yoke source checkout. Classification: verification venue/packaging mismatch, before any runtime assertion, with no source bisect needed. Retain this result and rerun at the same source and dependency pins with the exact Yoke checkout fixtures.
