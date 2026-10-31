---
id: 23cede21adca
kind: feature
title: 'tutor: guardrailed nudges with each student''s own agent'
seq: 3002
status: done
priority: p1
labels:
    - tutor
created: 2026-10-31T00:54:50.088596Z
weave: 393
assignee: muse-spark1.2
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
closed: 2026-10-31T02:04:41.741692Z
closed_by: s547-manager
---

tutor nudges: one `bashy chat` call with each person's own agent (--agent -> TUTOR_AGENT -> none); a guardrail prompt plus a post-filter keep output to hints and questions, never the work; context is the step title and done-when only; audience/tone come from the format. Test: a 'write it for me' step yields a hint, not a deliverable. Delivered in examples/tutor.
