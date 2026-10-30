---
id: 7dbbd367202b
kind: feature
title: 'S537.N2 notepad: convert human-edited text/Markdown to PDF, PNG, HTML for the agent'
seq: 2928
status: done
priority: p2
labels:
    - browser
    - desk
created: 2026-10-30T02:27:25.61041Z
weave: 354
assignee: agy-gemini3.8-flash
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T18:09:16.402683Z
closed_by: s537-manager
---

notepad (examples/desk/notepad, already merged as N1) — two parts:

1) FIRST, fix the save bug found in the macOS browser smoke (2026-10-30): in main.go page script, saving a NEW note shows the "Saved successfully" alert and then immediately does window.location.search='?id=…' (full reload wipes the alert); it also uses a blocking prompt() for the id. Fix: no prompt (server auto-generates the id); after a successful new save use history.replaceState to put ?id=<id> in the URL without reload, update currentId and the item header/links, and keep the alert visible. browser-test.mjs step "Save note" must pass unchanged (wait for [data-desk-alert] 'Saved successfully', then URL has ?id=). Add a Go test asserting the page script contains no prompt( and no window.location.search assignment (red before, green after).

2) Convert human-edited text/Markdown for the agent: `notepad export <id> --to pdf|png|html|txt [-o FILE]` (and the same via /api) returns the output path. Markdown->HTML, text/Markdown->PDF, PNG via golang.org/x/image. One new permissive pure-Go writer per format at most, license checked before adding (MIT/BSD/Apache only; name it in the submit note), added to the notepad module only. Red/green: one test per format (PDF starts %PDF, PNG decodes with image/png, HTML contains the heading).

Common: desk contract + kit at examples/desk/kit (read examples/desk/kit/README.md); keep contracttest green for the tool; focused red/green Go tests; gofmt clean (pre-push fmtcheck rejects otherwise); commit only your story files (do not commit KB.md or docs/todo); end with `bashy sprint submit 537 <story-id> --as <your binding> -m "<evidence>"`. Dragon: focused `go test` only, no podman. Spec (read-only): $HOME/projects/poc/dhnt/docs/sprint-537-bashy-desk-plan.md
