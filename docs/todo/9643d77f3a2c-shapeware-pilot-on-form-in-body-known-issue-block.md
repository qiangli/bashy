---
id: 9643d77f3a2c
kind: spike
title: 'Shapeware pilot on form: in-body known-issue block + one TBD spec, fresh-agent run'
seq: 3312
status: todo
priority: p2
labels:
    - shaping
created: 2026-11-25T10:17:49.05743Z
---

Follow-on to umbrella Sprint 712 (story b35b89620f88, plan only). This is its case (iii): a feature delivered without a new binary. Keep it KISS/MVP: prove the rod once, on form only.

1. Check that the Bash# block syntax (`~~~skill as guide`) accepts another block kind. If not, record the gap before changing anything.
2. Add one known-issue block INSIDE form.bsh (operator decision 2026-11-25: in-body, not a separate file), for bug 64ef2626: symptom, workaround (run form.bsh directly), class 'workaround active', expiry. It retires when 64ef2626 is fixed.
3. Add one TBD spec, from the shaped.json genie example 'Group unanswered required fields at the top', with a check under checks/ that fails today.
4. Fresh agent session, no hints: it should get past the bug in one step, and implement the TBD via `bashy shape wish` → verify → approve so that checks/ pass.

Measure: tool calls to get past the bug (baseline 2026-11-25: 5), whether checks pass, wall time. Results go back to Sprint 712. No new verbs and no framework.

## Addendum 2026-11-25: UI face and bashy-ui-v1 (operator agreed)

- UI is the human face of the Bash# coin: script for machines, skill for agents (`~~~skill as guide`), ui for human eyes (later audio/voice for ears). No new marker: the existing source-block forms are the embedding, `~~~ui as NAME !RUNNER … ~~~` inline or `embed ui "./FILE.yaml" as NAME !RUNNER` by reference. JSON/YAML is the default body (the fish); the runner is a registered command (form/page/chart).
- Step 3 writes the TBD spec as a `~~~feature` block (Intent / Structure / Acceptance; acceptance finds elements by label and role), and the form definition as a `~~~ui … !form` fence.
- Name the existing CLI↔serve contract **bashy-ui-v1** in the form README: the document envelope `{schema_version, kind, title, fields|blocks, actions}` (form fields ∪ page blocks vocabulary) plus the existing kit routes POST /api/action, GET /api/state, GET /api/export, GET /api/events. Extend kit/contracttest to the envelope.
- Known gap: a fence calls `RUNNER <verb> <file>`, but form expects `add --def FILE`. Close it together with 64ef2626 (accept a positional file).
- Acceptance tests that drive a browser run on the gate host, never the dev box.

## Addendum 2026-11-25 (b): raw view of a .bsh (operator agreed)

Viewing a `.bsh` as a page reuses existing pieces; there is no new verb.
- **Parse:** sh/syntax already parses blocks (`SourceBlock`: language, alias, runner, body). Projecting a face (`--skill` etc.) should use the parser, not the hand-rolled line loops in form.bsh and hello.bsh.
- **Convert:** add `.bsh` as a `bashy lens` input (yoke/pkg/lens), `lens -f bsh -t md`. The skill body becomes prose, the script becomes `bash` code blocks, `~~~feature` becomes a status table, and `~~~ui` becomes a block that renders as a static preview.
- **Render:** kit/render (goldmark + fence overrides, as charts already do) gets `feature` and `ui` block renderers; then `lens -f bsh -t md F | viewer`.
- **Rule: rendering is not running.** A raw view never executes runners; the ui face is a static preview with no working actions. HTML/CSS/JS is output only (the default fish, sanitized); the source stays `.bsh`.
- Reference artifact: the hello-world walkthrough (sprint 712 thread, 2026-11-25): hello.bsh with script/skill/ui/feature faces, verified-with-tbd release, field shaping, undo, upstream.

## Addendum 2026-11-25 (c): second case, a web app with a companion .bsh (operator agreed)

Pure Bash# must mesh with existing web technology, not replace it. Case 2 tests the **companion file** model on `examples/shaping/hello-world` (plain `public/index.html`, a static shaped app):
- Add `public/hello.bsh` as a companion carrying the skill, `~~~feature` specs with acceptance, and known issues. Add an optional `~~~ui` only if needed.
- Add one line to `index.html`: `<link rel="bashsharp" href="hello.bsh">`. The HTML is otherwise untouched; web developers keep their tools. Analogy: `.d.ts` beside `.js`, source maps beside minified code.
- Check: with no hints, a `bashy shape wish` (or Genie) on the app finds and uses the companion: it follows the skill, keeps the feature acceptance passing, and respects the known issues. Compare with the same wish and no companion.
- Measure: whether the companion was read, whether acceptance passes, and the steps/tool calls compared with the no-companion run.

Other mesh patterns are recorded as future input; they are not part of this pilot:
- a `~~~html`/`~~~ts as view` override fence built with the ecosystem's own tools via `~~~package`
- a `<bashy-face src=… face=ui>` web component consuming bashy-ui-v1
- the bashy-ui-v1 wire as the integration point for any framework, plus MCP
- the browser extension's Shape button reading the page's companion

## Addendum 2026-11-25 (d): no converter for the UI face (operator: no reinventing the wheel)

The UI face is a restricted profile of semantic HTML/CSS, so it needs no parser/converter. This SUPERSEDES addendum (b)'s `lens -f bsh -t md` and the `feature`/`ui` render overrides in kit/render for the UI face:
- **UI face:** the `~~~ui` body is restricted HTML, served as-is to the browser (visual). Audio walks the same semantic page with the CSS speech rules (`@media speech`), as screen readers already do.
- **What remains, all already existing:**
  - the face split, using the sh/syntax `SourceBlock` (replace the hand-rolled loops)
  - the profile check, by tightening the kit's bluemonday allowlist and keeping the self-only CSP: semantic elements only, no inline style or handlers, no page JS beyond kit.js
- **Raw view of a whole .bsh:** stays the notepad's plain-text view (worked 2026-11-25). A rendered view is not in the MVP.
- **Case 1 (form):** the form JSON stays as data; the form server already compiles it into kit HTML.
