---
id: 469f85f2bd84
kind: bug
title: S562 TypeScript and JavaScript explicit locale environment preservation
seq: 3086
status: assigned
priority: p0
created: 2026-11-05T04:03:45.481926Z
assignee: codex-gpt5.6-sol-w17
sprint: 578
sprint_id: 70ba48be-af89-5ab1-8a8d-42a15fc1ddbe
sprint_title: 'S562 follow-up: remaining fence corpus work'
---

E3 pinned TS ICU longtimezone row remains environmental because worker launch drops explicit LC_ALL/LANG while onlyTZ fix applied. Triage currenteb190 runtime. Preserve explicit locale vars narrowly forTS/JS launch/fingerprint inclabsentvs empty, retain sensitiveenvfilter sharedloader unchanged. Nativeoracle+interpreted+compiled Intl exact parity with matchedlocale fixtures. No broadinheritedenv or goldenweakening, no globalallowlist changes withoutreview.

Carried from S562 (operator-approved early close 2026-11-06); deferral is not PASS. Run/branch/evidence references: see existing story details and S562 board; no verified run or branch reference recorded.
