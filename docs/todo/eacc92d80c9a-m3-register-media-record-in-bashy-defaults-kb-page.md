---
id: eacc92d80c9a
kind: doc
title: 'M3 register media + record in bashy-defaults; kb pages: PDF from slides/images, narrated video'
seq: 3214
status: todo
priority: p2
labels:
    - media
    - kb
created: 2026-11-11T02:40:05.044776Z
sprint: 603
sprint_id: c808e55d-6ce2-5525-8829-96499861c90e
sprint_title: 'Agent-assisted PDF and video: small verbs, a media skill-script and kb pages any agent can follow'
---

Spec: dhnt-docs docs/sprint-603-master-execution-plan.md (M3). Supersedes dhnt-docs story 04cabb2fa1c4 (B4). Depends on M1's interface.

GOAL: (1) add `media` (examples/media/media.bsh) and `record` (examples/record/record.bsh) to DEFAULT_COMMANDS in examples/defaults/bashy-defaults.bsh, effects declared, with the defaults test updated; (2) two kb runbook pages in kb/defaults/pages: "make a PDF from slides or images" and "make a narrated video" (covering own-voice recording, camera via `record video`, terminal scenes via `cast record`). Each page names the verbs, the human browser steps, and per-OS install lines; no OS-only path; generic owner/member wording. TEST: examples/defaults go test.
