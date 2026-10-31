---
id: f8feda454ff8
kind: feature
title: 'tutor: prep session before class (shape content from scratch or from existing)'
seq: 3013
status: done
priority: p2
labels:
    - lern-psic
    - tutor
    - shaping
created: 2026-10-31T01:15:03.171087Z
weave: 399
assignee: agy-gemini3.8-flash
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
closed: 2026-10-31T02:31:06.533302Z
closed_by: s547-manager
---

Operator 2026-10-30: the plan session happens before class and is the Software Shaping part: the teacher and their agent create content + format from scratch, or copy and modify an existing one (last week, last year, another teacher's). tutor prep [--from PATH|sprint N] writes a draft content dir (format + steps + editions) the teacher edits; tutor apply turns it into the class sprint (student stories + teacher stories). Per-teacher, per-year variants are just copies with history (git); later adopts Sprint 526 shaping mechanisms, no dependency. Test: copy-and-modify from an existing content dir; apply produces the expected stories.


## Brief (manager s547-manager, 2026-10-30)
Common rules for every S547 worker:
- Skeleton is on bashy main (97a7f240): `examples/tutor/tutor.bsh` dispatches each verb to ONE module per story. Edit ONLY the files listed under "Your files"; if you truly need a change elsewhere (tutor.bsh, lib/common.py) keep it minimal and say so in your submit message.
- Contract: `examples/tutor/lib/common.py` (draft schema v1, step labels `step-NN` `timebox-Nm` `role-student|role-teacher` `phase-WORD` — bashy labels cannot contain ':' so the spec's `timebox:20m` is `timebox-20m`; run log `$XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl`; `bashy(...)` runner wrapper honouring BASHY_BIN).
- Python 3 stdlib only (no pip), Go stdlib only for server/. tutor never imports or calls klatch/form code; composition is by pipes/JSON at the shell.
- Tests: unittest under `examples/tutor/tests/test_<area>.py`, subclass `tests.helpers.TempBashy` (temp BASHY_HOME, coord, room, XDG and git repo; cwd = temp repo). Run `cd examples/tutor && python3 -m unittest discover -s tests -t . -v`. Never touch the live board, the class Klatch server/data, or Sprints 510/528. Red first, then green.
- Every verb accepts `--json`. No agent configured must still work (no crash, sensible fallback).
- Speed first: MVP, focused test, no extra features. Commit with the Sprint/Story/Story-ID trailers of THIS story.


Your files: `lib/prep.py`, `tests/test_prep.py`.
- `tutor prep DIR [--from PATH] [--goal G] [--minutes N] [--agent A] [--json]`: from scratch, writes DIR/format.md (copy of formats/class.md), DIR/content/<NN-slug>.md per planned step (using `tutor plan` internals from lib/plan.py — import, don't duplicate) with empty `~~~teacher` blocks, and DIR/draft.json; `--from PATH` copies an existing content dir (last week, another teacher) and refreshes draft.json from its content files. The teacher edits, then `tutor apply DIR/draft.json`. Draft steps get `content: "content/NN-slug.md"`. Teacher-owned steps (prepare, facilitate, review) included per the format.
- Tests: from-scratch with no agent; copy-and-modify from a fixture dir; apply on the result produces the expected student + teacher stories (TempBashy).
