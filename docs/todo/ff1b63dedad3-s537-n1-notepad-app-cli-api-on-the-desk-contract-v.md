---
id: ff1b63dedad3
kind: feature
title: 'S537.N1 notepad: app + CLI/API on the desk contract; view any content as text (PDF/HTML/MD/JSON)'
seq: 2927
status: done
priority: p2
labels:
    - browser
    - desk
created: 2026-10-30T02:27:25.318478Z
assignee: agy-gemini3.8-flash
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T17:35:03.473246Z
closed_by: s537-manager
---

notepad at examples/desk/notepad (command bashy notepad), was S536.5. View any content as text: txt, md, html (via JohannesKaufmann/html-to-markdown, MIT), json (pretty), PDF->text (ledongthuc/pdf, BSD-3); these deps are added to the notepad module only (check licenses in the submit note). Human edits text/Markdown in the page; the agent reads it back (`bashy notepad get <id>`), puts content (`put <id> -f FILE|-`), lists, shares. Notes stored under $BASHY_HOME/desk/notepad. Linux/macOS/Windows identical (paths via filepath, no shell-outs). Conversion to PDF/PNG/HTML is N2, NOT this story.

Common (all W2 stories): build on the K0 desk kit at examples/desk/kit (Go helpers, embedded CSS/JS UI kit, contracttest) via `require desk-kit v0.0.0` + `replace desk-kit => ../kit`; read examples/desk/kit/README.md and copy the pattern of examples/desk/kit/internal/fixture (memo tool: server, CLI, register.sh, dag.md, contract test usage via contracttest.Run). Desk contract: `bashy <tool> new|open|get|put|list|share|close` with --json; CLI verbs == server /api/... operations; atlas entry (help <tool> --format=mcp); every item has a stable id + loopback URL; `open <id>` prints id, URL, status only (never re-emits content); bind 127.0.0.1; data under $BASHY_HOME/desk/<tool>/; secret-typed input refused; Bash# front + pure-Go server + register.sh + dag.md (shapable, like examples/whiteboard); launcher tile via `bashy app add`. Done = kit contracttest passes for the tool + focused red/green Go tests + browser-test.mjs written (it runs on a test host later, not here). Dragon: focused `go test` only, no podman. Stay inside your own examples/desk/<tool> directory; any kit change goes in the submit note as a request, not an edit. Spec (read-only): $HOME/projects/poc/dhnt/docs/sprint-537-bashy-desk-plan.md
