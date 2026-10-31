---
id: 9d0a95cf258d
kind: feature
title: 'tutor: teacher + student editions of every content item (one source)'
seq: 3012
status: done
priority: p1
labels:
    - tutor
created: 2026-10-31T01:15:02.859418Z
weave: 392
assignee: agy-gemini3.8-flash
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
closed: 2026-10-31T02:04:39.664678Z
closed_by: s547-manager
---

tutor editions: one source per content item with private blocks; `tutor render --edition ROLE|shared` renders what each role sees, the shared edition never contains private content; `--format form` emits a form definition. Tests assert no private text leaks. Delivered in examples/tutor.
