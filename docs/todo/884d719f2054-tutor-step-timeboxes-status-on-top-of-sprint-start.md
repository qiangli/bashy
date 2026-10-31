---
id: 884d719f2054
kind: feature
title: 'tutor: step timeboxes + status (on top of sprint start/stop, no parallel lifecycle)'
seq: 3000
status: todo
priority: p0
labels:
    - lern-psic
    - tutor
created: 2026-10-31T00:54:49.360105Z
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

Operator 2026-10-30: a class starts exactly like a coding sprint — bashy sprint start N --owner <agent, usually Claude> --for 90m --instruction ... or the Start button on the /sprint/ UI; the managed manager acts as the tutor via the tutor skill. tutor adds only what sprint lacks: per-step timebox (label timebox:Nm), next/done for PEOPLE (todo status, since sprint claim needs an agent), extend per step, status --json and status --line (one line: 'Step 2/5 · Prototype sketch · 12:40 left · Ana, Ben'). Session clock stays sprint start/extend/stop. Run log under XDG data dir. Tests on a temp store.


## Brief (manager s547-manager, 2026-10-30)
Common rules for every S547 worker:
- Skeleton is on bashy main (97a7f240): `examples/tutor/tutor.bsh` dispatches each verb to ONE module per story. Edit ONLY the files listed under "Your files"; if you truly need a change elsewhere (tutor.bsh, lib/common.py) keep it minimal and say so in your submit message.
- Contract: `examples/tutor/lib/common.py` (draft schema v1, step labels `step-NN` `timebox-Nm` `role-student|role-teacher` `phase-WORD` — bashy labels cannot contain ':' so the spec's `timebox:20m` is `timebox-20m`; run log `$XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl`; `bashy(...)` runner wrapper honouring BASHY_BIN).
- Python 3 stdlib only (no pip), Go stdlib only for server/. tutor never imports or calls klatch/form code; composition is by pipes/JSON at the shell.
- Tests: unittest under `examples/tutor/tests/test_<area>.py`, subclass `tests.helpers.TempBashy` (temp BASHY_HOME, coord, room, XDG and git repo; cwd = temp repo). Run `cd examples/tutor && python3 -m unittest discover -s tests -t . -v`. Never touch the live board, the class Klatch server/data, or Sprints 510/528. Red first, then green.
- Every verb accepts `--json`. No agent configured must still work (no crash, sensible fallback).
- Speed first: MVP, focused test, no extra features. Commit with the Sprint/Story/Story-ID trailers of THIS story.

Your files: `lib/run.py`, `tests/test_run.py`.
Steps are the card's todos carrying a `step-NN` label (create them in tests directly with `bashy todo add ... --sprint N --owner P --label step-01,timebox-10m,role-student`). People are not agents, so step status uses `bashy todo status ID doing|done`; the session clock is the sprint's own (`bashy sprint start/extend/stop`; read state/deadline from `bashy sprint show N` — find a JSON form via `bashy adapt --json -- ...` or parse; if the card has no clock, the session is 'not started').
Verbs (`--json` everywhere):
- `start N [--for 90m] [--owner AGENT]`: thin wrapper — if the sprint is not already running, call `bashy sprint start N --for ... --owner ...` (pass through; if it refuses, print its error); then mark step-01 doing and log `{"ts","event":"step_start","step":id}`.
- `next N`: mark the current doing step done (if any) and the next pending step doing; log both.
- `done N STEP [-m EVIDENCE]`: STEP = id or index; mark done, log `{"event":"done","evidence":...}`; does not auto-advance.
- `extend N STEP --by 5m`: log `{"event":"extend","by_s":300}` (affects that step's timebox only; session extend stays `bashy sprint extend`).
- `status N [--json|--line]`. JSON shape (consumed by serve pages and chat glue — keep exactly):
  `{"sprint":N,"title":str,"clock":{"state":"running|not-started|stopped","deadline":iso|null,"remaining_s":int|null},"current":{"index":i,"total":n,"id":str,"title":str,"owners":[..],"timebox_s":int,"elapsed_s":int,"remaining_s":int,"overdue":bool}|null,"steps":[{"index","id","title","owner","role","status","timebox_s","started_at","done_at","overdue","evidence"}]}`.
  Elapsed comes from the run log step_start; timebox = label minutes + extends. `--line`: `Step 2/5 · Prototype sketch · 12:40 left · Ana, Ben` (`· 3:10 over` when overdue; `Not started · 5 steps` / `Done · 5/5 steps` edge cases). One line, no ANSI.
- Tests: injectable clock (env `TUTOR_NOW` ISO timestamp honoured by run.py) to assert remaining/overdue deterministically; next/done/extend round trip; --line format.
