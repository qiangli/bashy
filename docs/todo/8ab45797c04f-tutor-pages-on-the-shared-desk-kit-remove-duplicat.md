---
id: 8ab45797c04f
kind: refactor
title: 'tutor: pages on the shared desk kit + remove duplicate start (audit)'
seq: 3024
status: done
priority: p0
labels:
    - tutor
created: 2026-10-31T04:34:25.711791Z
weave: 407
assignee: codex-gpt6-astra
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
closed: 2026-10-31T05:04:16.679621Z
closed_by: s547-manager
---

tutor pages on the shared desk kit, plus duplicate removal: the display, owner and participant pages consume examples/desk/kit (tokens, cookies, headers, styles) instead of local copies; `tutor start` is removed because the sprint's own clock already covers it; session steps live in the user's personal todo store (never the current repo); tests isolate HOME; the example uses placeholder names only; README states that session data is private and local; the design note moved to examples/tutor/docs. Delivered in examples/tutor.
