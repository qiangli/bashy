---
id: 9643d77f3a2c
kind: spike
title: 'Shapeware pilot on form: in-body known-issue block + one TBD spec, fresh-agent run'
seq: 3312
status: done
priority: p1
labels:
    - shaping
created: 2026-11-25T10:17:49.05743Z
assignee: agy-gemini4-carbon
sprint: 721
sprint_id: 69fc2c51-8faf-58e7-83ac-f27007837ae7
sprint_title: 'Shapeware pilot on form: spec v0 layout, one-step launch'
closed: 2026-11-25T21:01:23.40442Z
closed_by: claude-opus5.5
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

## Addendum 2026-11-25 (d): restricted HTML AND the converter, not either/or (operator)

No reinventing the wheel: when the UI face is written as restricted semantic HTML/CSS, it needs no conversion. The converter from addendum (b) STAYS part of the solution. Both paths coexist:
- **Direct path:** a `~~~ui` body in restricted HTML is served as-is (visual). Audio walks the same semantic page with `@media speech` rules, as screen readers do.
- **Converter path (addendum (b), kept):**
  - `lens -f bsh -t md` plus the `feature`/`ui` renderers in kit/render, for faces that are data (`~~~feature` specs, JSON/YAML `~~~ui`/form definitions)
  - the rendered raw view of a whole .bsh
  - representations the browser does not give for free (Markdown/PDF export, terminal, MCP elicitation, audio)
- **Shared foundation:**
  - the face split, using the sh/syntax `SourceBlock` (replace the hand-rolled loops)
  - the restricted-HTML profile check: a tightened kit bluemonday allowlist plus the self-only CSP; semantic elements only, no inline style or handlers, no page JS beyond kit.js. The converter emits this profile too, so both paths meet in the same HTML.

## Addendum 2026-11-25 (e): baseline, launching a form with today's `bashy form` (operator request)

**Goal (operator):** evaluate a shapeware-enabled `form` against today's. Launching a form should take a few steps, ideally one, with little probing.

**Task:** the agent had 9 decisions for the operator (umbrella Sprint 712 review) and was asked "show with bashy form". The agent was Claude Opus 5.5 on the dev box, with installed bashy (98a460a). The steps below are exactly as they happened. Tool calls are numbered; the elapsed time was not measured.

| # | Step | Result | Kind |
|---|---|---|---|
| 1 | `ls examples/`, read `examples/contact.json` (schema), `bashy commands show form` | schema found | probe |
| 2 | wrote the definition JSON; `bashy form add --def FILE --title T` | **FAIL** `unknown flag --def` (bug 64ef2626) | defect |
| 3 | ran `bashy --bashsharp examples/desk/form/form.bsh add …` directly (the workaround recorded in this pilot) | **FAIL** the server rebuild fetched `examples/desk/kit/v0.5.0` and `examples/desk/chart/v0.1.3`, unknown revisions (local Go proxy) | defect |
| 4–8 | `bashy commands show form` (text: no argv); `bashy adapt --json -- …` (nothing); `ls ~/.bashy`; `bashy commands list`; `bashy commands --help`; `bashy commands show form --yaml` | found `exec: bashy --bashsharp <checkout>/form.bsh`: the registered command runs the same source as step 3 | probe |
| 9–10 | read `form.bsh`: `build()` and the dispatch | found the cached server `~/.cache/bashy/form/form` (built 11-24) | probe |
| 11 | `~/.cache/bashy/form/form -addr 127.0.0.1:18107 add --def FILE --title T` | **OK** form created | launch |
| 12 | `curl` the URL | 200 | check |
| 13 | background `form wait ID` (no key) | **FAIL** "Join this form first": the key is not resolved when the binary runs directly (README: "resolved automatically") | defect |
| 14 | `show --key K` | pending | check |
| 15 | background `wait` with `FORM_KEY` | ended after the default 5 min; `--timeout` is not in `-h` | defect (doc) |
| 16 | operator: "you did not show the form yet". `add` only prints a URL; the agent never opened it | agent miss; `open URL` | miss |
| 17–19 | looked for the timeout: `wait --help` (nothing), `-h`, grep the README and `server/cli.go` | found `--timeout` (default 5m) in source | probe |
| 20 | `wait --timeout 4h` | answered | wait |

**Baseline:** about 20 tool calls before the operator saw the form (the first page showed only at step 16). 11 were probes, 4 hit product defects (flags; the source rebuild; key resolution; the undocumented wait timeout), and 1 was an agent miss (the page was never opened). The operator also had to prompt twice.

**What a shapeware-enabled form must make one step** (the acceptance targets for this pilot):
1. `SKILL.md`'s standard run block names one command that **creates and opens** the form: for example `bashy form add --def FILE --open`, or `ask`. The skill must say "open it for the human"; printing a URL is not showing it.
2. The run uses the **trusted core**: a prebuilt, digest-pinned server (`shaped.json`), never a source rebuild on use. Step 3's failure class disappears.
3. Every defect above is a known issue (symptom → action) until fixed, so an agent that hits one recovers in one step with no probing. Example: "`unknown flag --def` → run `<path to core> add --def …`".
4. Waiting is one step: `wait --timeout DURATION` documented in the skill, and the key resolved automatically for the creating agent.
5. Measure: tool calls and probes to the first shown form, and to the first answer read. Target: 1 call to show it, 1 to read it. Compare with this baseline on the same task.

## Sprint 721 P1 instruction (conductor, 2026-11-25) — implementation only

Scope is the approved Sprint 721 acceptance, NOT every addendum above. Out of scope for this run: addendum (a) bashy-ui-v1 envelope + contracttest, (b) `lens -f bsh` converter / raw view, (c) hello-world companion case, (d) restricted-HTML allowlist. The measurement (3 arms x 3 cold runs) is P2, planned separately; do not run agents.

Start from current bashy origin/main (form flags and the four addenda are fixed: 64ef2626). Read umbrella `docs/shapeware-spec-v0.md` v0.1 (sections 2-12 and Appendix A, the `hello` example, as the template). Then, in `examples/desk/form` only:
1. SKILL.md with `metadata.type: shapeware` (no top-level `type`), min-band L2, body order per section 4.1, the standard run block by intent (section 4.3): ONE command that creates AND opens a form for the human (add `--open` to `form add`/`ask`; printing a URL is not showing it), plus `wait --timeout DURATION` to read the answer.
2. Known issues in the SKILL.md body (section 4.2): seed exactly one realistic known issue (symptom -> action, class, expiry) so the pilot can test recovery; it must not hide a real open defect.
3. `dag.md` lifecycle (section 5a): source, build, test, install, run targets that work on a fresh HOME (bashy dag now runs in-process).
4. `features.yaml` (section 5) with ONE `tbd` feature taken from the shaped.json genie example "Group unanswered required fields at the top"; `checks/acceptance.bsh` (fails today because of that tbd) and `checks/candidate.bsh` per section 6.
5. `shaped.json`: core digest pinning the prebuilt server (section 9) so use never rebuilds from source.
6. Keep `form.bsh`, the registered command and existing behaviour working; existing desk tests stay green.

Verify: `GOWORK=off go test ./...` in examples/desk/form and the examples/desk module tests; `bashy skill validate`-style frontmatter check (metadata.type read via yoke cb1a95c4); a fresh-HOME run of `bashy dag -f examples/desk/form/dag.md test`; `checks/acceptance.bsh` fails ONLY on the tbd. Record the commands and results. No new verbs beyond `--open`, no framework, no browser tests on the dev box.
