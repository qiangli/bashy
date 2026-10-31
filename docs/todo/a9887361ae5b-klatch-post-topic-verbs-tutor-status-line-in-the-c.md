---
id: a9887361ae5b
kind: feature
title: klatch post/topic verbs + tutor status line in the class chat (composed)
seq: 3007
status: todo
priority: p0
labels:
    - lern-psic
    - tutor
    - klatch
created: 2026-10-31T01:05:44.258613Z
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

Operator 2026-10-30: chat is the class surface this weekend (Sprint 528). Unix composition, no coupling: (1) bashy klatch gains generic verbs 'post GROUP TEXT' and 'topic GROUP TEXT' (stdin '-'), usable by anything; (2) tutor prints 'status --line'; (3) glue = a pipe/loop in the tutor skill or a dag step, e.g. bashy sprint watch N | while read _; do bashy tutor status N --line | bashy klatch topic class -; done, plus milestone posts. tutor never imports klatch and klatch never knows tutor. Bot account via bashy klatch provision. Verify the header shows on web + phone; if not visible enough, file a Klatch status-panel UI follow-up. Tests: fake Databag server (pattern examples/klatch/test_provision.py) + one live check on the class server before Nov 1.
