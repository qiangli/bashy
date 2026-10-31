---
id: f8feda454ff8
kind: feature
title: 'tutor: prep session before class (shape content from scratch or from existing)'
seq: 3013
status: todo
priority: p2
labels:
    - lern-psic
    - tutor
    - shaping
created: 2026-10-31T01:15:03.171087Z
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

Operator 2026-10-30: the plan session happens before class and is the Software Shaping part: the teacher and their agent create content + format from scratch, or copy and modify an existing one (last week, last year, another teacher's). tutor prep [--from PATH|sprint N] writes a draft content dir (format + steps + editions) the teacher edits; tutor apply turns it into the class sprint (student stories + teacher stories). Per-teacher, per-year variants are just copies with history (git); later adopts Sprint 526 shaping mechanisms, no dependency. Test: copy-and-modify from an existing content dir; apply produces the expected stories.
