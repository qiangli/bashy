---
id: c4fba141e7a9
kind: feature
title: 'tutor: owner controls tempo — pause/resume, adjust time, jump/skip, reorder'
seq: 3033
status: todo
priority: p0
labels:
    - tutor
created: 2026-10-31T21:04:06.229375Z
assignee: claude-sonnet5.5
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

Operator-approved feature (tutor only): the session format is a starting plan; the owner controls the tempo live.

## Brief (manager s547-manager) — examples/tutor ONLY (hard rule); small; tests per control
State model: everything is a run-log event (`$XDG_DATA_HOME/bashy/tutor/<N>.jsonl`) plus the existing todo labels; `status` folds the events. No changes outside examples/tutor.
CLI (lib/run.py; register.sh flags; skill text one line each):
1. `tutor pause N` / `tutor resume N` — events pause/resume; while paused the current step's elapsed time does not advance; `status --json` gains `"paused": true|false` (top level) and `--line` shows "Paused · Step i/n · title".
2. `tutor time N [STEP] --add 5m | --add -1m` (current step by default; any signed duration; replaces/extends today's `extend`, keep `extend` as an alias only if it costs nothing — no compatibility shims otherwise). Timebox never below 1 minute.
3. `tutor goto N STEP` — make STEP (index or id) the current step: the previous current step returns to todo (unless done), STEP becomes doing; works backwards and forwards. `tutor skip N` — mark the current step skipped (event; label `step-skipped`; counted as finished for progress, shown as skipped) and start the next pending step.
4. `tutor move N STEP --up|--down|--to K` — reorder upcoming (not done) steps by rewriting their `step-NN` labels via `bashy todo edit --label/--unlabel` (personal store, as today) and a `reorder` event; done/current steps keep their place.
Owner page (server, desk kit only — no new styling): Pause/Resume toggle, buttons -1 / +1 / +5 min and a small custom minutes input, a step list where each row has Go (jump) and ↑/↓ for upcoming rows, and Skip. All actions go through the existing owner-key-protected POST path and call the CLI verbs above (server holds no second state). Display + participant pages show "Paused" while paused and the updated remaining time.
Tests: one focused test per control in tests/test_run.py (pause freezes elapsed with TUTOR_NOW; resume continues; time ±; goto back and forward; skip; move up/down/to and that labels are rewritten); Go tests for the new owner actions (owner key required, participant token refused); browser-test: owner page shows the controls and Display shows "Paused" after pause. Keep all existing gates and closure greps green. Commit with this story's trailers; submit with the exact CLI examples.

## Continue (manager review of run #419, commit aa4d5574 in refs/salvage/abandoned-419) — finish, do not redo
Start from aa4d5574 (cherry-pick it; tests already green: tutor 64, server race). Required before merge:
1. REMOVE the committed binary `examples/tutor/server/tutor-server` (10.9 MB) from the commit, and add `examples/tutor/server/tutor-server` to examples/tutor/.gitignore (create it) so a build never lands again.
2. Labelled links + owner page first (missing): lib/flow.sh (demo/go) prints exactly `<Owner role title> (private): URL`, `Display (projector): URL`, `Phone for <name>: URL` and opens the OWNER URL; test_flows.py asserts the labels and opener order.
3. Teaching principle (missing): formats/session.md guidance says timers are guides, never limits; participants can browse all steps and mark Done any time. Verify overtime text is "+M:SS over" with no error/red class on all three pages (add an assertion if absent).
4. Presenter keys: confirm Owner AND Display pages handle ArrowLeft/ArrowRight, PageUp/PageDown, Space; browser-test presses ArrowRight then ArrowLeft on the owner page and asserts the Display's current step follows.
Keep everything inside examples/tutor; one clean commit on top (or amend) with this story's trailers; submit.

## Fix (manager live check of merged #420, bashy 63160908) — demo cleanup race
Repro on Dragon: `cd examples/tutor && TUTOR_OPEN=echo timeout -s INT 25 bashy --bashsharp ./tutor.bsh demo --port 18098` → links print correctly (Owner/Display/Phone, owner opened first) but on Ctrl-C: `rm: cannot remove '<tmp>': Directory not empty`, then `==> demo FAILED (exit 1)` and the throwaway dir is LEFT BEHIND (it contained home/). lib/flow.sh kills and waits for $SERVE_PID, but the server's own children (status polling `bashy tutor ...`) or the real server process behind the wrapper still write under the throwaway home.
Fix in lib/flow.sh only: stop the whole server process tree (e.g. run serve in its own process group and kill the group, or kill children then wait), then remove the dir robustly (retry rm a few times briefly). On Ctrl-C the demo must exit 0 with no "FAILED" line if possible (trap INT → clean exit), and leave no throwaway dir. Test in test_flows.py: SIGINT → exit code 0/130-without-FAILED, throwaway dir gone, port closed. Commit with this story's trailers; submit.
