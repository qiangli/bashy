---
id: 6238ebc797ee
kind: bug
title: 'S537.D1b desk shell consistency bugs: one top-bar action set, notepad status badge, gallery check'
seq: 2967
status: done
priority: p1
labels:
    - desk
created: 2026-10-30T19:18:35.316789Z
weave: 372
assignee: claude-sonnet5.5
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T19:30:08.654825Z
closed_by: s537-manager
---

Bug on accepted story D1 (911a3e70, bashy fb356c2c), found by the supervisor's review of the gallery (examples/desk/gallery/shots). Fixed inside Sprint 537.
1. Top-bar actions differ between tools: notepad and chart have Copy link, brainstorm (check whiteboard and form too) do not; notepad repeats Copy link at the bottom. ONE shell action set for every tool (kit shell owns it), no duplicate actions in tool bodies.
2. Notepad shows the kit's all-statuses demo strip under the header; show only the item's current status badge.
3. Gallery check: capture.mjs (or a Go/Playwright test) asserts every tool page has the same top-bar action set and exactly one status badge; red on a planted extra/missing action.
(The chart SVG type-scale bug is fixed in N3 step 2, which owns the ChartSVG path — do not touch examples/desk/chart render code or kit/render.)
Base: bashy main 0bf7ebc5 or later (D2 verbs). Gate: gofmt + go vet + go test in kit, notepad, form/server, chart, whiteboard/server, brainstorm/server; Playwright browser tests; regenerate gallery shots light+dark.
