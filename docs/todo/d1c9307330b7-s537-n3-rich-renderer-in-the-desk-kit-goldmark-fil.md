---
id: d1c9307330b7
kind: feature
title: S537.N3 rich renderer in the desk kit (goldmark, file-type embeds, safe) + notepad live preview
seq: 2961
status: done
priority: p1
labels:
    - desk
created: 2026-10-30T18:25:43.570346Z
weave: 373
assignee: muse-conductor
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T19:46:22.014892Z
closed_by: s537-manager
---

Operator-approved scope add (2026-10-30, "A, add both to 537 now"). p1, ~2-3h, strong worker. Dirs: examples/desk/kit + examples/desk/notepad. Starts AFTER N2 merged (done: bc572ccd).

- Replace blackfriday with goldmark (MIT, pure Go): GFM tables, task lists, strikethrough, footnotes, autolinks, definition lists, TOC, code highlighting (permissive pure-Go only; license checked before adding).
- Renderer lives in the kit: notepad preview, viewer and HTML export share ONE implementation.
- Embeds by file type, standard Markdown, degrading gracefully: ![](x.csv|.tsv) -> HTML table (header, first N rows, download link); png/jpg/svg -> img; mp4/webm -> <video controls>; pdf -> browser built-in PDF viewer; fenced ```chart (widget + data) -> server-side SVG reusing the C1 chart renderer; fenced ```csv -> inline table.
- Assets local under $BASHY_HOME/desk/<tool>/items/<id>/assets/, served same-origin; remote http(s) embeds OFF by default, opt-in per item.
- Safety: no raw HTML pass-through (if ever added: bluemonday-sanitized); strict CSP; view page runs only kit JS; rendered view issues no mutating API calls.
- Notepad: Markdown textarea + server-rendered live preview, edit/split/view toggle. No WYSIWYG, no inline image editing. HTML export = the same rendered view, self-contained (assets inlined).
- Red/green tests: each embed type, remote-embed default-off, raw-HTML/script injection neutralised, contract test still green.

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

SCOPE UPDATE (manager, 2026-10-30 ~12:15 PDT, after D1/D2 were filed — operator cohesion directive):
- Step 1 (this run, now): examples/desk/kit/render ONLY — the renderer, file-type embeds, sanitizer/CSP safety tests. Emit SEMANTIC CLASSES ONLY (e.g. desk-md, desk-table, desk-code, desk-embed-*); write NO CSS and do not touch kit.css — S537.D1 (run 366) owns kit.css + the app shell and styles your classes. Page() links kit CSS/JS only.
- Do NOT edit examples/desk/notepad now: S537.D2 (run 365) is renaming the notepad CLI/API verbs (new->add, get->show, put->set) and moving imports onto bashy lens.
- Step 2 (notepad live preview) starts after D2 merges: the manager will tell you via weave say; rebase onto main then and wire the preview using D2's verbs.

STEP 1 MERGED (manager, 12:25 PDT): bashy 287e0306 = examples/desk/kit/render (goldmark + bluemonday + chroma, embeds, sanitizer). D1 merged as fb356c2c: kit.css styles `.desk-content` (+ table.desk-table, .desk-embed-*), kit.js has a client `deskRender` Markdown SUBSET used by notepad preview, form help, brainstorm cards, whiteboard notes.

STEP 2 (this run) — ONE renderer, cohesive with D1:
a) Wrapper class: render emits `desk-content` (D1's class), not `desk-md`. Keep desk-table / desk-embed-* / desk-code / desk-toc; check each has a D1 style in kit.css, add the missing styles to kit.css using D1's tokens only (no hex, no font-family; contracttest conformance must stay green).
b) Remove the built-in chart fallback (render/chart.go, a second chart renderer duplicating C1). ChartSVG stays a hook; examples/desk/chart exports the fence renderer and each tool that embeds charts sets render.ChartSVG at startup. When unset, the fence renders as a code block plus a desk-embed-error note. Chart output uses D1's data palette.
c) Kit render endpoint: a small kit HTTP helper (e.g. POST <tool>/api/render, Markdown in, sanitized HTML out) that every tool mounts; kit.js `deskRender` keeps its name and signature but delegates to that endpoint, so the client-side Markdown subset is deleted. Callers (form help, brainstorm cards, whiteboard notes, notepad preview) are unchanged.
d) Notepad live preview through the endpoint — ONLY after D2 (run 365) merges; the manager will `weave say` you. Until then do a)–c) in kit, chart and the per-tool mount lines; if D2 has not merged by the time a)–c) are done, submit a)–c) and the manager relaunches d).
Red/green: render tests updated for desk-content; a test that deskRender's endpoint returns sanitized HTML (script stripped); contracttest green on all tools; go vet/test green in kit, chart, notepad, form/server, whiteboard/server, brainstorm/server.

UPDATE 12:40 PDT: D2 MERGED (bashy 0bf7ebc5): verbs add/show/set/list/rm, schema_version, error object {code,message}, notepad imports via bashy lens; contracttest kitConformance + cliConformance. Step d is unblocked — rebase onto origin/main. ADD to b: chart SVG text must follow the kit type scale in the ONE ChartSVG path (chart page text too large, notepad embed labels too small, legend reads like debug 'line ·'); regenerate golden SVGs.

=== REWORK (manager, 12:55 PDT) — THIS RUN STARTS HERE ===
Prior step-2 work is preserved at refs/salvage/abandoned-369 (commit 3fb113d9, "one renderer - kit endpoint, C1 chart fence export, D1 cohesion"). Step 1: `git cherry-pick 3fb113d9` onto current origin/main (which has D2 0bf7ebc5: verbs add/show/set/list/rm, schema_version, error object {code,message}, kitConformance + cliConformance). Resolve conflicts keeping D2's verbs and error shape.
Then finish — these are REQUIRED for acceptance:
1. REGRESSION FIX: kit.js deskRender now POSTs to <tool>/api/render, but only chart and form mount kit.ServeRender. Mount it in notepad, whiteboard/server and brainstorm/server too (and set render.ChartSVG = fence.RenderFence where charts can appear). Add a contracttest check: every page that loads kit.js answers POST /api/render with sanitized HTML (red if a tool forgets to mount it).
2. Notepad live preview through the endpoint (step d), using D2's verbs.
3. Chart text follows the kit type scale in the ONE fence renderer (chart page text was too large, notepad-embed labels too small, legend read like debug 'line ·'); regenerate goldens.
Note: D1b (bug story, run 372) is concurrently editing the kit shell top bar and notepad status badge — do not change top-bar actions or status badges; rebase on main again before submitting if D1b merged.
Gate: gofmt + GOWORK=off go vet + go test in kit, notepad, form/server, chart, whiteboard/server, brainstorm/server; run each tool's browser test if present.
