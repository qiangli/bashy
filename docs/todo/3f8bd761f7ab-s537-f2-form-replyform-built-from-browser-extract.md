---
id: 3f8bd761f7ab
kind: feature
title: 'S537.F2 form replyform: built from browser extract, per-field copy buttons + status badges, fed by guard refusals; optional in-page overlay'
seq: 2929
status: todo
priority: p2
labels:
    - browser
    - desk
created: 2026-10-30T02:27:25.901822Z
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, whiteboard, brainstorm) on one contract + UI kit'
---

replyform on the F1 form engine (examples/desk/form, merged; read examples/desk/form/README.md for the definition schema/answer store). Purpose: when an agent cannot (or must not) fill a web form, it hands the human a replyform: one row per page field with the agent's proposed value, a per-field copy button (kit CopyButton, "Copied" state) and a fill status badge (filled-by-agent, failed, needs-human, pending).

Input = `bashy browser extract` JSON, schema FROZEN by Sprint 536 (yoke candidate bf28337f; additive, existing fields retained):
- the extract result's `data` is a JSON STRING; decode it; it holds `elements`: rows {index, tag, text, attrs{...}, visible, box, line, frame_id?, stable_id, label, selector, name}.
- Key rows by `stable_id` (fallback `selector`); show `label` (fallback name, then attrs.placeholder); preserve `selector` VERBATIM (frame selectors are qualified); never rely on `index` after the page changes.
- Guard refusal / failed fill results carry `outcome_class`, `reason` (a guard reason code such as field:label-ssn or value:luhn — never the value), `content`, `error`. A refused or failed field becomes a needs-human (refused) or failed row whose reason is shown; its value is NEVER shown or stored (the guard refused it because it is sensitive).
CLI: `bashy form reply --extract FILE|- [--values FILE] [--refusals FILE]` (or `form new --from-extract`) creates the replyform and prints id/URL; the human marks rows done on the page; `form get <id> --json` returns per-field status keyed by stable_id. Only input/select/textarea-like rows become fields (skip buttons/links).
The in-page overlay is OPTIONAL and OUT of scope unless trivial — skip it.
Red/green tests with checked-in fixture JSON you write from the schema above: extract->rows keyed by stable_id with labels, refusal row shows reason and no value, failed fill row status, selector preserved verbatim incl. a frame-qualified one, open hides content, contract test green.
Common: keep the form contracttest and all F1 tests green; gofmt clean; commit only examples/desk/form files (not KB.md/docs/todo); finish with `bashy sprint submit 537 3f8bd761f7ab --as <your binding> -m "<evidence>"`. Dragon: focused go test only, no podman, no browser. Spec (read-only): $HOME/projects/poc/dhnt/docs/sprint-537-bashy-desk-plan.md
