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
