---
id: 117da8bb8ca6
kind: enhancement
title: 'tutor: one-command demo and go (no exports, no sprint number, opens the page)'
seq: 3031
status: assigned
priority: p0
labels:
    - tutor
created: 2026-10-31T06:00:40.141277Z
weave: 411
assignee: claude-sonnet5.5
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

Operator 2026-10-31 (blocks close): the try flow is too complicated even for an engineer; make it one command, inside examples/tutor only.

## Brief (manager s547-manager) — examples/tutor ONLY (hard rule), reuse existing bashy pieces, no new design
1. `bashy tutor demo [--port P]` — one command, nothing to set up: create a throwaway dir (mktemp -d) and export HOME, BASHY_HOME, BASHY_COORD_DIR, BASHY_ROOM_DIR, BASHY_APPS_DIR, XDG_DATA_HOME, XDG_CACHE_HOME inside it plus BASHY_SPRINT_ANNOUNCE=0 (nothing reaches the user's real stores or board); plan a sample goal for alice,bob (template, no agent), apply, next (step 1 running), start `serve` in the background (`--json` output gives display/owner/participant links; LAN IP is already in them), wait for /healthz, open the Display page in the browser with bashy's existing opener (find it: e.g. `bashy open URL` / the launcher's opener; fall back to `open`/`xdg-open`/`start`), then print exactly: the Display URL, ONE phone link (alice) and "Ctrl-C to stop". On Ctrl-C/EXIT: stop the server and rm -rf the throwaway dir. Keep the build cache in the user's real cache dir so the server does not rebuild every time (set XDG_CACHE_HOME back for the build or pass TUTOR_SERVER_BIN).
2. `bashy tutor go "GOAL" --people a,b [--minutes 60] [--format F] [--agent A] [--port P]` — real session in one command in the user's own home: plan -> apply (read the sprint number from apply --json; the user never types it) -> next -> serve in background -> open the Display page -> print the owner link (keep private) + one join link per person + "Ctrl-C to stop serving (session data stays)". Prints the sprint number once for later `bashy tutor status N`.
3. Skill text (~~~skill in tutor.bsh): a user can tell their agent "start a tutor session for <goal> with alice and bob" → the agent runs `bashy tutor go "<goal>" --people alice,bob`; "show me a demo" → `bashy tutor demo`. Register both verbs and their flags in register.sh (registry schema), keep verify-duality green.
4. Tests (focused): demo and go with a stub opener (env TUTOR_OPEN=<script that records the URL>) and a short-lived run: assert healthz up, the Display URL opened, exactly one phone link printed with a non-loopback host when a LAN address exists, step 1 running, and that after SIGINT the server is gone and (demo) the throwaway dir is removed and the real HOME stores are unchanged. Tests isolate HOME as today.
Gates: tutor suite, server go -race, verify-duality, browser-test; greps stay 0 (cohort/host terms, names other than alice/bob/carol, ages, dates). Commit with this story's trailers; submit with the exact one-line commands.
5. (operator add-on) The flows live in examples/tutor/dag.md as targets — `demo` and `go` (params GOAL, PEOPLE, MINUTES, plus optional FORMAT/AGENT/PORT) — because bashy dag is how bashy orchestrates. `bashy tutor demo|go` only run those targets (thin dispatch in tutor.bsh), so `bashy dag examples/tutor/dag.md demo` works too. Keep the existing rehearsal/parity targets. Tests cover both entry points.

## Continue (manager) — run #411 hit the 48m runtime cap; work preserved at refs/salvage or commit 5793c57e
Start from 5793c57e (fetch it from the canonical bashy repo: `git fetch $HOME/projects/poc/dhnt/bashy refs/salvage/abandoned-411` or cherry-pick 5793c57e; exclude KB.md). State: dag targets, lib/flow.sh, dispatch, register and test_flows.py exist; the tutor suite passes (58).
BUG (manager repro): `cd examples/tutor && TUTOR_OPEN=echo timeout -s INT 90 bashy --bashsharp ./tutor.bsh demo --port 18095` prints NOTHING and is still running after 90 s (exit 124). Expected within ~20 s: Display URL, one phone link, "Ctrl-C to stop"; then Ctrl-C stops the server and removes the throwaway dir. Likely suspects: server build in the throwaway XDG_CACHE_HOME (rebuild every time — reuse the user's real cache or a prebuilt TUTOR_SERVER_BIN), output buffered/swallowed by the dag runner, or a foreground wait before printing. Fix it, prove it by running that exact command (and `go` the same way in a temp HOME), keep tests green, commit with this story's trailers (one clean commit on top of the wip is fine), submit. Stay inside examples/tutor.
