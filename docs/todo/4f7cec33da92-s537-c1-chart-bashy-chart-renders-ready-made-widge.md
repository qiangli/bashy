---
id: 4f7cec33da92
kind: feature
title: 'S537.C1 chart: bashy chart renders ready-made widgets for OTel metric types (counter/gauge/updown/histogram) + line/bar/table/stat from JSON/CSV/OTLP-JSON; pure-Go SVG; shapable templates; not a dashboard'
seq: 2935
status: assigned
priority: p2
labels:
    - desk
created: 2026-10-30T02:54:01.296365Z
weave: 353
assignee: codex-gpt6-astra
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, whiteboard, brainstorm) on one contract + UI kit'
---

chart at examples/desk/chart (command bashy chart): quick, nice stats; deliberately NOT a dashboard app. Input: JSON, CSV, NDJSON or OTLP-JSON metrics (file or stdin). Renders one page of ready-made widgets, server-side SVG in pure Go (no new deps, no CDN, works offline, same look light/dark). Widgets map to OTel metric types: Counter/Sum -> total + rate-over-time line; Gauge -> stat tile + sparkline; UpDownCounter -> line; Histogram/ExponentialHistogram -> distribution bars + p50/p95/p99 tiles; plus generic line, bar, table, stat. Agents pick a widget (`--widget`) or tweak a shipped template (templates are files, shapable). Snapshot by default; optional `--refresh` re-reads a file or a loopback URL. No query language, no accounts, no layout editor. Red/green: golden SVG tests per widget type + one OTLP-JSON fixture.

Common (all W2 stories): build on the K0 desk kit at examples/desk/kit (Go helpers, embedded CSS/JS UI kit, contracttest) via `require desk-kit v0.0.0` + `replace desk-kit => ../kit`; read examples/desk/kit/README.md and copy the pattern of examples/desk/kit/internal/fixture (memo tool: server, CLI, register.sh, dag.md, contract test usage via contracttest.Run). Desk contract: `bashy <tool> new|open|get|put|list|share|close` with --json; CLI verbs == server /api/... operations; atlas entry (help <tool> --format=mcp); every item has a stable id + loopback URL; `open <id>` prints id, URL, status only (never re-emits content); bind 127.0.0.1; data under $BASHY_HOME/desk/<tool>/; secret-typed input refused; Bash# front + pure-Go server + register.sh + dag.md (shapable, like examples/whiteboard); launcher tile via `bashy app add`. Done = kit contracttest passes for the tool + focused red/green Go tests + browser-test.mjs written (it runs on a test host later, not here). Dragon: focused `go test` only, no podman. Stay inside your own examples/desk/<tool> directory; any kit change goes in the submit note as a request, not an edit. Spec (read-only): $HOME/projects/poc/dhnt/docs/sprint-537-bashy-desk-plan.md
