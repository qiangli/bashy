---
id: 8ab45797c04f
kind: refactor
title: 'tutor: pages on the shared desk kit + remove duplicate start (audit)'
seq: 3024
status: assigned
priority: p0
labels:
    - tutor
created: 2026-10-31T04:34:25.711791Z
weave: 406
assignee: claude-sonnet5.5
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

Operator 2026-10-30 scope reset: tutor follows the same look, feel and layout as whiteboard/brainstorm/desk (shared desk kit + launcher design system); change nothing in those tools; do not reimplement what bashy already has.

## Brief (manager s547-manager, 2026-10-30 ~22:30 PDT)
Base: bashy main be8dec26 (generic tutor merged; examples/klatch restored to pre-S547 — do NOT touch examples/klatch, whiteboard, brainstorm or examples/desk; reuse only).

A. Pages on the shared desk kit (`examples/desk/kit`: kit.go, kit.css, kit.js, README contract). Make `examples/tutor/server` consume it exactly as `examples/brainstorm/server` and the desk tools do (`require desk-kit v0.0.0` + `replace desk-kit => ../../desk/kit`, GOWORK=off builds still work from inside the umbrella): same header/layout/tokens (kit.css + launcher design system), same security mechanics from kit.go (48-hex tokens, owner key header + constant-time compare, HttpOnly SameSite=Strict participant cookies, strict JSON decode, same-origin POST check, CSP/nosniff/no-store headers, `/healthz`, `-data/-addr/-prefix` flags) — delete tutor's own copies of anything the kit provides. No new styling: if a needed widget is missing from the kit, use the closest existing kit element and list the gap in your submit (do not edit the kit). Pages stay: display (countdown of the current step), owner page (next/extend), per-participant page (own step, Done + optional evidence link). Labels come from status JSON `roles` (unchanged). Update browser-test.mjs (desktop + 390px phone) and Go tests.

B. Duplicate audit removals (operator rule: keep only what bashy lacks):
- Remove the `tutor start` verb and the run-log `session_start` fallback: the session clock is the sprint's own (`bashy sprint start N --owner AGENT --for 90m`); the first `tutor next N` marks step 1 doing. Update registry (register.sh), skill text, README, dag.md and tests accordingly.
- Keep: plan/apply (format→timed steps; apply composes sprint add/person add/todo add), next/done/extend (per-person step state; sprint claim needs an agent), status --json/--line (per-step view; clock read from the sprint's box), render, nudge (one `bashy chat` call + guardrail), prep, serve/join (participant pages without accounts).

Acceptance: `git grep -n -E "func .*Token|Content-Security-Policy|SameSite" examples/tutor/server` shows only kit calls (no local reimplementation); `git grep -n -w start examples/tutor/tutor.bsh examples/tutor/register.sh` has no tutor `start` verb; tutor suite, `GOWORK=off go test -race ./...` in server, verify-duality.py and browser-test.mjs all green (Playwright: PLAYWRIGHT_MODULE=/private/tmp/d1pw/node_modules/playwright/index.mjs CHROMIUM_PATH=$HOME/Library/Caches/ms-playwright/chromium_headless_shell-1243/chrome-headless-shell-mac-arm64/chrome-headless-shell). Commit with this story's trailers; submit with the grep outputs and a screenshot path of the phone page.

D. The moved doc (examples/tutor/docs/plan-generic-formats.md) must be scrubbed: no cohort, school, date or host names (the quoted acceptance grep pattern counts — describe it generically, e.g. "forbidden cohort/host names"). Nothing tutor-related stays in bashy/docs/ (story records in docs/todo are the store and stay).
