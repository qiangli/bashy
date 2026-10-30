---
id: e5193ee0230d
kind: feature
title: 'S537.F1 form engine + ad-hoc forms: agent defines JSON/YAML form, human fills on the page, agent reads answers back'
seq: 2932
status: todo
priority: p1
labels:
    - desk
created: 2026-10-30T02:45:42.70162Z
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, whiteboard, brainstorm) on one contract + UI kit'
---

form engine + ad-hoc forms at examples/desk/form (command bashy form). An agent defines a form in JSON or YAML (fields: text, textarea, choice, multi, yes-no, file; required; one recommended option per choice/multi shown with a mark); the human fills it on the loopback page using the kit field components; the agent reads answers back as JSON keyed by field id (`bashy form get <id> --json`, and `bashy form wait <id>` or polling for status pending/answered). Partial saves survive reload. Fields typed secret are refused with a clear error pointing to bashy ask. Engine is the base for replyform (F2) and questionnaire (F3): keep the definition schema and answer store reusable, document them in examples/desk/form/README.md.

Common (all W2 stories): build on the K0 desk kit at examples/desk/kit (Go helpers, embedded CSS/JS UI kit, contracttest) via `require desk-kit v0.0.0` + `replace desk-kit => ../kit`; read examples/desk/kit/README.md and copy the pattern of examples/desk/kit/internal/fixture (memo tool: server, CLI, register.sh, dag.md, contract test usage via contracttest.Run). Desk contract: `bashy <tool> new|open|get|put|list|share|close` with --json; CLI verbs == server /api/... operations; atlas entry (help <tool> --format=mcp); every item has a stable id + loopback URL; `open <id>` prints id, URL, status only (never re-emits content); bind 127.0.0.1; data under $BASHY_HOME/desk/<tool>/; secret-typed input refused; Bash# front + pure-Go server + register.sh + dag.md (shapable, like examples/whiteboard); launcher tile via `bashy app add`. Done = kit contracttest passes for the tool + focused red/green Go tests + browser-test.mjs written (it runs on a test host later, not here). Dragon: focused `go test` only, no podman. Stay inside your own examples/desk/<tool> directory; any kit change goes in the submit note as a request, not an edit. Spec (read-only): $HOME/projects/poc/dhnt/docs/sprint-537-bashy-desk-plan.md
