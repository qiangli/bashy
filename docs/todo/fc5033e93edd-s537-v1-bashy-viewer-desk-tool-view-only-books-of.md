---
id: fc5033e93edd
kind: feature
title: 'S537.V1 bashy viewer desk tool: view-only books of rendered items (Markdown or template+JSON), self-contained export'
seq: 2962
status: done
priority: p1
labels:
    - desk
created: 2026-10-30T18:25:43.776508Z
weave: 370
assignee: claude-opus5.5
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T19:42:24.784736Z
closed_by: s537-manager
---

Operator-approved scope add (2026-10-30, "A, add both to 537 now"). p1, ~1-1.5h. New dir examples/desk/viewer. May run in parallel with N3 once the kit renderer API is fixed; final integration after N3.

- View-only by construction: verbs new|open|get|list|share|close + append (add an item to a book); NO edit/put; open prints id/URL/status only.
- Uses the kit renderer; items from Markdown or from a template + JSON data; templates are shapable files (status, plan-summary, recap, close-out); a status template can read live sprint card data (bashy sprint show N --json) so agents write only the narrative.
- Book: ordered items per sprint (survives manager handoffs) or per session; first/prev/next/last, index page, jump-to-id, "changed since previous" highlight.
- Export: self-contained HTML; rich PDF via browser print (human) / bashy browser (agent).
- Name: `viewer` (not `view` = vim read-only in userland).
- Passes contracttest (view-only variant) + focused red/green tests + browser-test.mjs.
- G1 then documents: human edits text -> notepad; human reads report/recap/document -> viewer; in chat give a 2-4 line summary + viewer link instead of the full report. Notepad preview + viewer join the W5 per-OS round-trip.

## Fixed renderer API (manager, binding for N3 and V1)

New additive subpackage `examples/desk/kit/render` (package render). N3 implements it; V1 codes against it (V1 may stub with a local shim until N3 merges, then switch imports - final integration after N3):

```go
type Options struct {
    AssetDir    string // item asset dir ($BASHY_HOME/desk/<tool>/items/<id>/assets); relative embeds resolve here only
    AssetURL    string // same-origin URL prefix under which AssetDir is served (e.g. "/assets/<id>/")
    AllowRemote bool   // http(s) embeds; default false -> rendered as plain links
    Inline      bool   // self-contained export: inline assets as data: URIs
}
func Markdown(src []byte, opt Options) (html []byte, err error) // HTML fragment, sanitized, no raw HTML pass-through
func Page(title string, body []byte) []byte                     // full page with strict CSP, kit CSS/JS only
```
Chart fences call the C1 renderer (examples/desk/chart) via a small exported func; if that needs a chart-module change, keep it additive.


## Operator clarification (2026-10-30, via s537-supervisor): DATA SEPARATE FROM TEMPLATE; AGENT NEVER GENERATES HTML
Pipeline: agent gives data JSON (+ short Markdown prose fields) or a Markdown body -> viewer fills a shipped shapable template (Go text/template -> Markdown with embeds: ```chart blocks, ![](x.csv), live sources like `bashy sprint show N --json`) -> kit renderer (render.Markdown, goldmark, no raw HTML) -> sanitized HTML.
1. Each template declares its data shape (small JSON schema file beside it); --data is validated and refused with a clear error, never rendered broken.
2. Logic-light text/template only; values escaped; no code exec or remote fetch from templates.
3. `bashy viewer append <book> --template status --data new.json` adds a report; the page highlights changes vs the previous item.
Red/green required: raw HTML/script in data neutralised; bad data rejected.

SCOPE UPDATE (manager, 2026-10-30 ~12:20 PDT, operator cohesion directive): V1 launches AFTER S537.D1 (design language: kit tokens + one app shell) and S537.D2 (CLI/API conformance) merge. Build the viewer on D1's shell and kit content styles (no own palette, no hex colours or font-family outside the kit). Use D2's standard subverbs and envelope: add/show/set/list/rm (NOT new/get/put), every common option, schema_version bashy-viewer-v1, HTTP API = CLI 1:1, exit 2 for unsupported. Render through N3's examples/desk/kit/render. contracttest's kit-conformance and CLI-conformance checks must pass.
