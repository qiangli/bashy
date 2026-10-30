---
id: 6114bd067b65
kind: feature
title: 'S537.F3 form questionnaire: numbered Q1..Qn persistent page, agent waits/polls JSON answers, reopen by id (no content re-emit)'
seq: 2930
status: todo
priority: p2
labels:
    - browser
    - desk
created: 2026-10-30T02:30:30.225976Z
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, whiteboard, brainstorm) on one contract + UI kit'
---

form questionnaire on the F1 engine (examples/desk/form, merged; read examples/desk/form/README.md for the definition schema and answer store). `bashy form ask` (or a questionnaire mode of `form new`): numbered questions Q1..Qn rendered on one persistent page; the human answers in any order with partial saves surviving reload; the agent waits/polls (`form wait <id>`, `form get <id> --json`) and gets answers as JSON keyed by question id (q1..qn) with per-question status; follow-up rounds APPEND questions (Q n+1..) to the same id without repeating earlier ones; `form open <id>` re-surfaces id/URL/status only (no content re-emit — token saving is the point). Choice questions show a recommended mark. Secret-typed questions refused (bashy ask/vault stays the path). Red/green: append-round test, partial-save/reload test, open-hides-content test, answers keyed by stable question id after append.

Common: desk contract + kit at examples/desk/kit (read examples/desk/kit/README.md); keep contracttest green for the tool; focused red/green Go tests; gofmt clean (pre-push fmtcheck rejects otherwise); commit only your story files (do not commit KB.md or docs/todo); end with `bashy sprint submit 537 <story-id> --as <your binding> -m "<evidence>"`. Dragon: focused `go test` only, no podman. Spec (read-only): $HOME/projects/poc/dhnt/docs/sprint-537-bashy-desk-plan.md
