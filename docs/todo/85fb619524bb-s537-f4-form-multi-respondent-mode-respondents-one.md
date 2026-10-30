---
id: 85fb619524bb
kind: feature
title: 'S537.F4 form multi-respondent mode: respondents one|many, per-respondent + aggregate results, poll preset'
seq: 2966
status: done
priority: p1
labels:
    - desk
created: 2026-10-30T18:58:52.874606Z
weave: 371
assignee: codex-gpt6-sol
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T19:30:40.858931Z
closed_by: s537-manager
---

Operator-approved (2026-10-30, verbatim): "add it to 537 as a form story. agreed. this is the missing part of form for using with many respondants which could be used as quentionarire basically".
Dir: examples/desk/form. Starts AFTER F2b + D2 merge; uses D2 verbs (add/show/set, common options, schema_version) and D1's shell.
Scope:
- One definition field `respondents: one|many`, valid for ad-hoc forms AND questionnaires.
- many: one response per participant (share link; editable until close); optional anonymous; optional close time.
- Results per respondent + aggregate (tally for choice/multi/yes-no, list for text); live for the owner, optionally visible to respondents. Agent reads both via `form show <id> --json`.
- Poll = preset: `form add --poll 'Lunch?' --options a,b,c`.
Red/green: 3 respondents, 1 vote each -> tally correct; re-vote replaces the earlier vote; closed form refuses a response. contracttest stays green.
Also covered by G1 guidance and the W5 per-OS round-trip.
