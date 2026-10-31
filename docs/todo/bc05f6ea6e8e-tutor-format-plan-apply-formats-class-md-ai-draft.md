---
id: bc05f6ea6e8e
kind: feature
title: 'tutor: format + plan + apply (formats/class.md, AI draft, template fallback)'
seq: 2999
status: todo
priority: p0
labels:
    - lern-psic
    - tutor
created: 2026-10-31T00:54:48.956336Z
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

examples/tutor: formats/class.md editable session format (software shaping surface); prompts/plan.md; tutor plan GOAL --minutes --people --format → JSON draft (validated), no agent → template split; tutor apply DRAFT creates the sprint card (epic) + people-owned todos with timebox labels via plain bashy sprint add / todo add / person add. The draft can also be produced by whichever agent the teacher uses (Claude/Codex/Muse) from the tutor skill. Tests: stub agent + no-agent fallback in temp BASHY_HOME. Spec docs/lern-psic/tutor.md
