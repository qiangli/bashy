---
id: 9d0a95cf258d
kind: feature
title: 'tutor: teacher + student editions of every content item (one source)'
seq: 3012
status: todo
priority: p1
labels:
    - lern-psic
    - tutor
created: 2026-10-31T01:15:02.859418Z
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

Operator 2026-10-30: like teacher/student editions of a textbook. One source file per step/lesson (content/<step>.md); teacher-only parts in fenced blocks (~~~teacher: answers, rubric, facilitation notes, hint ladder), mirroring Bash# ~~~skill duality, so the two editions never drift and may be identical. bashy tutor render STEP --edition student|teacher [--format md|json]. Composes with desk tools, no coupling: questions → 'tutor render --edition student --format form | bashy form add --def -' (prefilled where the format says so); answers come back via 'bashy form show --json' and the teacher's agent checks them against the teacher edition; nudges may use the answer key to choose a hint but never reveal it. Teacher-side stories: every class sprint also gets teacher-owned todos (prepare, facilitate, review) next to the student stories, as in common practice. Test: render both editions, assert the student edition contains no ~~~teacher content; form-def round trip with a fake form server.


## Brief (manager s547-manager, 2026-10-30)
Common rules for every S547 worker:
- Skeleton is on bashy main (97a7f240): `examples/tutor/tutor.bsh` dispatches each verb to ONE module per story. Edit ONLY the files listed under "Your files"; if you truly need a change elsewhere (tutor.bsh, lib/common.py) keep it minimal and say so in your submit message.
- Contract: `examples/tutor/lib/common.py` (draft schema v1, step labels `step-NN` `timebox-Nm` `role-student|role-teacher` `phase-WORD` — bashy labels cannot contain ':' so the spec's `timebox:20m` is `timebox-20m`; run log `$XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl`; `bashy(...)` runner wrapper honouring BASHY_BIN).
- Python 3 stdlib only (no pip), Go stdlib only for server/. tutor never imports or calls klatch/form code; composition is by pipes/JSON at the shell.
- Tests: unittest under `examples/tutor/tests/test_<area>.py`, subclass `tests.helpers.TempBashy` (temp BASHY_HOME, coord, room, XDG and git repo; cwd = temp repo). Run `cd examples/tutor && python3 -m unittest discover -s tests -t . -v`. Never touch the live board, the class Klatch server/data, or Sprints 510/528. Red first, then green.
- Every verb accepts `--json`. No agent configured must still work (no crash, sensible fallback).
- Speed first: MVP, focused test, no extra features. Commit with the Sprint/Story/Story-ID trailers of THIS story.

Your files: `lib/render.py`, `tests/test_render.py`, `content/example-step.md` (one sample item).
- Source item `content/<step>.md`: Markdown; teacher-only parts in fenced blocks opened by a line `~~~teacher` (optionally `~~~teacher answers` etc.) and closed by `~~~`. Optional front-matter `title:`, `done_when:`, `questions:` list.
- `tutor render FILE|STEP --edition student|teacher [--format md|json|form] [--json]`: student = source minus every ~~~teacher block (and nothing of their contents, including in front-matter fields named `answer*`); teacher = everything with the blocks shown as a `> Teacher:` section. `--format form` prints a bashy form definition JSON for the questions (check `bashy form --help` / `bashy form add --help` for the --def schema and match it) — tutor only prints; the user pipes it to `bashy form add --def -`.
- Tests: student edition contains no teacher text (assert with a unique marker string), teacher edition contains it, identical editions when no blocks; form-def output validates against what `bashy form add --def -` expects (use a fake/temporary form store only, never a live one).
