---
id: 85074ac78e4f
kind: feature
title: 'tutor: register + duality + README; Nov 1 dry run'
seq: 3003
status: todo
priority: p1
labels:
    - lern-psic
    - tutor
created: 2026-10-31T00:54:50.533064Z
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

register.sh (commands add/skill add/app add), verify-duality.py, README (class/meeting/solo, make your own format = software shaping example); dry run Nov 1 Week 2 agenda; link outcome to Sprint 528.


## Brief (manager s547-manager, 2026-10-30)
Common rules for every S547 worker:
- Skeleton is on bashy main (97a7f240): `examples/tutor/tutor.bsh` dispatches each verb to ONE module per story. Edit ONLY the files listed under "Your files"; if you truly need a change elsewhere (tutor.bsh, lib/common.py) keep it minimal and say so in your submit message.
- Contract: `examples/tutor/lib/common.py` (draft schema v1, step labels `step-NN` `timebox-Nm` `role-student|role-teacher` `phase-WORD` — bashy labels cannot contain ':' so the spec's `timebox:20m` is `timebox-20m`; run log `$XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl`; `bashy(...)` runner wrapper honouring BASHY_BIN).
- Python 3 stdlib only (no pip), Go stdlib only for server/. tutor never imports or calls klatch/form code; composition is by pipes/JSON at the shell.
- Tests: unittest under `examples/tutor/tests/test_<area>.py`, subclass `tests.helpers.TempBashy` (temp BASHY_HOME, coord, room, XDG and git repo; cwd = temp repo). Run `cd examples/tutor && python3 -m unittest discover -s tests -t . -v`. Never touch the live board, the class Klatch server/data, or Sprints 510/528. Red first, then green.
- Every verb accepts `--json`. No agent configured must still work (no crash, sensible fallback).
- Speed first: MVP, focused test, no extra features. Commit with the Sprint/Story/Story-ID trailers of THIS story.


Your files: `register.sh`, `verify-duality.py`, `README.md`, `dag.md`, the `~~~skill` block in `tutor.bsh` (final wording).
- register.sh: pattern examples/brainstorm/register.sh — `commands add tutor` (exec bashy --bashsharp tutor.bsh, synopsis, verbs enum, --json flag), `skill add` from `--skill` + `--contract`, `app add tutor --port 18093`. Run it against a TEMP BASHY_HOME in the test, never the live home.
- verify-duality.py: pattern examples/brainstorm/verify-duality.py.
- README: class / meeting / solo examples; "start a class like a coding sprint" (`bashy sprint start N --owner <agent> --for 90m --instruction 'run today's session with the tutor skill'`); chat composition (`chat-status.sh`); "make your own format" (copy formats/class.md = Software Shaping example); teacher/student editions.
- dag.md: the Nov 1 dry run as a dag (plan → apply → start → next/done → status --line) in a temp BASHY_HOME with no agent and with a stub agent. Record the dry-run output as evidence in the submit message. Linking the outcome to Sprint 528 is done by the manager.
