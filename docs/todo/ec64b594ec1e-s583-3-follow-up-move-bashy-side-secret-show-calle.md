---
id: ec64b594ec1e
kind: chore
title: 'S583.3 follow-up: move bashy-side secret show callers to handles (klatch.bsh text, docs) and document secret exec --stdin'
seq: 3207
status: todo
priority: p2
labels:
    - boundary
    - secrets
created: 2026-11-10T06:08:42.69511Z
---

From yoke EVIDENCE-aafea94af0b8.md: secret show/env now refuse agents and non-TTY stdin. Update bashy/examples/klatch/klatch.bsh (line 19 text and any resolution path) and bashy docs/skills that show piping secret show, to the handle forms: secret exec NAME -- CMD (env) and secret exec --stdin NAME -- CMD (stdin). Add the stdin form to the bashy secret skill/help. Rod not fish: no new verbs.
