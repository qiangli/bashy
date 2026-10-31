---
id: bc05f6ea6e8e
kind: feature
title: 'tutor: format + plan + apply (formats/class.md, AI draft, template fallback)'
seq: 2999
status: done
priority: p0
labels:
    - tutor
created: 2026-10-31T00:54:48.956336Z
weave: 395
assignee: claude-opus5
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
closed: 2026-10-31T02:04:02.766914Z
closed_by: s547-manager
---

tutor plan/apply: an editable session format (Markdown + front-matter) drives a draft of timed, role-owned steps — from an agent (`bashy chat`) or a template split when no agent is set; `apply` creates the sprint card, people and step todos via plain `bashy sprint add` / `person add` / `todo add`. Tests: stub agent + no-agent fallback in a temporary BASHY_HOME. Delivered in examples/tutor.
