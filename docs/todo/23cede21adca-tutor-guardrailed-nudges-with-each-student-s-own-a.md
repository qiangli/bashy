---
id: 23cede21adca
kind: feature
title: 'tutor: guardrailed nudges with each student''s own agent'
seq: 3002
status: done
priority: p1
labels:
    - lern-psic
    - tutor
created: 2026-10-31T00:54:50.088596Z
weave: 393
assignee: muse-spark1.2
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
closed: 2026-10-31T02:04:41.741692Z
closed_by: s547-manager
---

prompts/nudge.md hints/questions only; agent resolution --agent → TUTOR_AGENT → teacher default; bashy chat --json one-shot; context = step title + done-when only. Test: write-it-for-me request yields hints, not deliverables.


## Brief (manager s547-manager, 2026-10-30)
Common rules for every S547 worker:
- Skeleton is on bashy main (97a7f240): `examples/tutor/tutor.bsh` dispatches each verb to ONE module per story. Edit ONLY the files listed under "Your files"; if you truly need a change elsewhere (tutor.bsh, lib/common.py) keep it minimal and say so in your submit message.
- Contract: `examples/tutor/lib/common.py` (draft schema v1, step labels `step-NN` `timebox-Nm` `role-student|role-teacher` `phase-WORD` — bashy labels cannot contain ':' so the spec's `timebox:20m` is `timebox-20m`; run log `$XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl`; `bashy(...)` runner wrapper honouring BASHY_BIN).
- Python 3 stdlib only (no pip), Go stdlib only for server/. tutor never imports or calls klatch/form code; composition is by pipes/JSON at the shell.
- Tests: unittest under `examples/tutor/tests/test_<area>.py`, subclass `tests.helpers.TempBashy` (temp BASHY_HOME, coord, room, XDG and git repo; cwd = temp repo). Run `cd examples/tutor && python3 -m unittest discover -s tests -t . -v`. Never touch the live board, the class Klatch server/data, or Sprints 510/528. Red first, then green.
- Every verb accepts `--json`. No agent configured must still work (no crash, sensible fallback).
- Speed first: MVP, focused test, no extra features. Commit with the Sprint/Story/Story-ID trailers of THIS story.

Your files: `prompts/nudge.md`, `lib/nudge.py`, `tests/test_nudge.py`.
- `tutor nudge N [--person P] [--agent A] [--json]`: find P's current doing step (todos on sprint N with `step-NN` label and owner P; status doing; without --person, the first doing step). Agent resolution: `--agent` → `$TUTOR_AGENT` → none. With none: print a built-in generic question from the prompt file's fallback list (no crash). With an agent: one-shot `bashy chat --agent A -m <prompts/nudge.md filled with step title + done-when ONLY> --json`, read `.output`.
- Guardrail: prompts/nudge.md instructs hints and questions only, never the solution; plus a post-filter in nudge.py that rejects outputs that look like deliverables (code fences, more than ~80 words, full essays) and replaces them with a generic question; students are 13+ — keep tone age-appropriate. Never send other students' data or teacher-edition content to the agent.
- Tests: stub agent via `BASHY_BIN` fake (answers `chat` with a canned envelope, execs real bashy otherwise): (1) "write it for me" style step + agent returning a full code solution → output is a hint/question, not the code; (2) agent returns a good hint → passed through; (3) no agent → fallback question. Assert the prompt sent contains only title + done-when.
