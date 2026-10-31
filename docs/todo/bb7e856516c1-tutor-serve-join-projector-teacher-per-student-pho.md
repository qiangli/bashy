---
id: bb7e856516c1
kind: feature
title: 'tutor: serve + join (projector, teacher, per-student phone pages)'
seq: 3001
status: todo
priority: p1
labels:
    - lern-psic
    - tutor
created: 2026-10-31T00:54:49.739141Z
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

Go stdlib server port 18093: projector countdown, private teacher link, per-student join links (no accounts), mobile-first; bashy tutor join URL on student machine; done with evidence link to desk tools. Race tests + browser-test.mjs projector+phone; second LAN host join.


## Brief (manager s547-manager, 2026-10-30)
Common rules for every S547 worker:
- Skeleton is on bashy main (97a7f240): `examples/tutor/tutor.bsh` dispatches each verb to ONE module per story. Edit ONLY the files listed under "Your files"; if you truly need a change elsewhere (tutor.bsh, lib/common.py) keep it minimal and say so in your submit message.
- Contract: `examples/tutor/lib/common.py` (draft schema v1, step labels `step-NN` `timebox-Nm` `role-student|role-teacher` `phase-WORD` — bashy labels cannot contain ':' so the spec's `timebox:20m` is `timebox-20m`; run log `$XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl`; `bashy(...)` runner wrapper honouring BASHY_BIN).
- Python 3 stdlib only (no pip), Go stdlib only for server/. tutor never imports or calls klatch/form code; composition is by pipes/JSON at the shell.
- Tests: unittest under `examples/tutor/tests/test_<area>.py`, subclass `tests.helpers.TempBashy` (temp BASHY_HOME, coord, room, XDG and git repo; cwd = temp repo). Run `cd examples/tutor && python3 -m unittest discover -s tests -t . -v`. Never touch the live board, the class Klatch server/data, or Sprints 510/528. Red first, then green.
- Every verb accepts `--json`. No agent configured must still work (no crash, sensible fallback).
- Speed first: MVP, focused test, no extra features. Commit with the Sprint/Story/Story-ID trailers of THIS story.


Your files: `server/` (Go stdlib module `examples/tutor/server`, own go.mod like examples/brainstorm/server), `server/run.sh` (replace the stub), `browser-test.mjs`, `tests/test_serve.py` if useful.
- Data source: the server shells out to `bashy tutor status N --json` (shape fixed by story 884d719f2054, see its brief) every few seconds, and to `bashy tutor done N STEP -m EVIDENCE` / `next N` for actions. It holds no second copy of session state.
- `tutor serve N [--addr 0.0.0.0:18093] [--prefix P]`: prints a projector link, a private teacher link (random key) and one join link per step owner (per-person random token, no accounts), like brainstorm. Pages: projector (big countdown, current step, owners, overdue in red; no tokens shown); teacher (all steps, next/extend buttons, key-gated); student `/s/<token>` (mobile-first: own current step, countdown, done-when, Done button with optional evidence link to a desk tool such as notepad/form/chart URL). Tokens stored under `$XDG_DATA_HOME/bashy/tutor/<N>-serve.json` (0600).
- `tutor join URL`: student side; opens/prints the student page URL and, with `--done -m EVIDENCE`, posts Done to the teacher's server. `tutor stop`: stop a server started for N.
- Use the launcher/desk visual conventions of examples/brainstorm (same CSS tokens) — bashy is one cohesive tool.
- Tests: `go test -race ./...` with a fake `BASHY_BIN` that returns canned status JSON (token isolation: a student token cannot act for another step; teacher key required for next/extend); `browser-test.mjs` projector + 390px phone viewport (pattern examples/brainstorm/browser-test.mjs).
