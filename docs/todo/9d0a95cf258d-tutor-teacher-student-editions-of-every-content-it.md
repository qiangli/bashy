---
id: 9d0a95cf258d
kind: feature
title: 'tutor: teacher + student editions of every content item (one source)'
seq: 3012
status: todo
priority: p1
labels:
    - lern-psic
    - tutor
created: 2026-10-31T01:15:02.859418Z
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

Operator 2026-10-30: like teacher/student editions of a textbook. One source file per step/lesson (content/<step>.md); teacher-only parts in fenced blocks (~~~teacher: answers, rubric, facilitation notes, hint ladder), mirroring Bash# ~~~skill duality, so the two editions never drift and may be identical. bashy tutor render STEP --edition student|teacher [--format md|json]. Composes with desk tools, no coupling: questions → 'tutor render --edition student --format form | bashy form add --def -' (prefilled where the format says so); answers come back via 'bashy form show --json' and the teacher's agent checks them against the teacher edition; nudges may use the answer key to choose a hint but never reveal it. Teacher-side stories: every class sprint also gets teacher-owned todos (prepare, facilitate, review) next to the student stories, as in common practice. Test: render both editions, assert the student edition contains no ~~~teacher content; form-def round trip with a fake form server.
