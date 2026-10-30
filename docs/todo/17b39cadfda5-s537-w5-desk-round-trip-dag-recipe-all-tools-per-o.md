---
id: 17b39cadfda5
kind: chore
title: S537.W5 desk round-trip dag recipe (all tools, per OS) + evidence record
seq: 2962
status: done
priority: p1
labels:
    - desk
created: 2026-10-30T20:00:48.514868Z
weave: 379
assignee: codex-gpt6-sol
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T20:33:24.485071Z
closed_by: s537-manager
---

Planned wave W5 (plan docs/sprint-537-bashy-desk-plan.md, Master execution plan row W5): "freeze, one dag recipe round-trips all five tools per OS, one claim per host, one evidence record". This story delivers the RECIPE and its local proof; the manager runs it on novidesign (macOS), noviwin1 (Windows) and a dhnt-ephemeral droplet (Linux).
Deliver examples/desk/dag.md (type: dag; Bash# / bashy commands only, no OpenSSH, portable on macOS/Windows/Linux) with tasks:
- build: build every desk tool (notepad, form, chart, viewer, whiteboard, brainstorm) with the tool's own .sh/dag.
- roundtrip: per tool, start its server on a free loopback port, then via the CLI with --json: add -> show -> (set where supported) -> list -> open -> close; assert schema_version bashy-<tool>-v1 on every JSON reply, exit 2 + error {code,message} on an unsupported verb, POST /api/render returns sanitized desk-content HTML; form: one respondents=many poll with 3 votes (tally), re-vote replaces, closed refuses; viewer: add --template status --data, export --to html; notepad: import one HTML via bashy lens; chart: one ```chart fence through notepad /api/render; stop each server. Exit non-zero on any failure, print one summary line per tool.
- gallery: run examples/desk/gallery/capture.mjs when a browser is available (skip with a clear note otherwise) — light+dark shots of every tool incl. viewer.
- evidence: write one JSON evidence record (os, arch, bashy version, per-tool pass/fail, durations) to a path given by $EVIDENCE or ./desk-roundtrip.json.
Local proof: run `bashy dag roundtrip` (and build) on this Mac; attach the evidence JSON summary in the submit message. Do not touch tool code except to fix a real bug the recipe exposes (report it).
