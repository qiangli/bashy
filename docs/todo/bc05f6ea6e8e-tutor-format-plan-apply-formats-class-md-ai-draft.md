---
id: bc05f6ea6e8e
kind: feature
title: 'tutor: format + plan + apply (formats/class.md, AI draft, template fallback)'
seq: 2999
status: todo
priority: p0
labels:
    - lern-psic
    - tutor
created: 2026-10-31T00:54:48.956336Z
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

examples/tutor: formats/class.md editable session format (software shaping surface); prompts/plan.md; tutor plan GOAL --minutes --people --format → JSON draft (validated), no agent → template split; tutor apply DRAFT creates the sprint card (epic) + people-owned todos with timebox labels via plain bashy sprint add / todo add / person add. The draft can also be produced by whichever agent the teacher uses (Claude/Codex/Muse) from the tutor skill. Tests: stub agent + no-agent fallback in temp BASHY_HOME. Spec docs/lern-psic/tutor.md


## Brief (manager s547-manager, 2026-10-30)
Common rules for every S547 worker:
- Skeleton is on bashy main (97a7f240): `examples/tutor/tutor.bsh` dispatches each verb to ONE module per story. Edit ONLY the files listed under "Your files"; if you truly need a change elsewhere (tutor.bsh, lib/common.py) keep it minimal and say so in your submit message.
- Contract: `examples/tutor/lib/common.py` (draft schema v1, step labels `step-NN` `timebox-Nm` `role-student|role-teacher` `phase-WORD` — bashy labels cannot contain ':' so the spec's `timebox:20m` is `timebox-20m`; run log `$XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl`; `bashy(...)` runner wrapper honouring BASHY_BIN).
- Python 3 stdlib only (no pip), Go stdlib only for server/. tutor never imports or calls klatch/form code; composition is by pipes/JSON at the shell.
- Tests: unittest under `examples/tutor/tests/test_<area>.py`, subclass `tests.helpers.TempBashy` (temp BASHY_HOME, coord, room, XDG and git repo; cwd = temp repo). Run `cd examples/tutor && python3 -m unittest discover -s tests -t . -v`. Never touch the live board, the class Klatch server/data, or Sprints 510/528. Red first, then green.
- Every verb accepts `--json`. No agent configured must still work (no crash, sensible fallback).
- Speed first: MVP, focused test, no extra features. Commit with the Sprint/Story/Story-ID trailers of THIS story.

Your files: `formats/class.md`, `prompts/plan.md`, `lib/plan.py`, `tests/test_plan.py`.
- `formats/class.md`: Markdown + YAML-ish front-matter (parse it with a tiny stdlib parser: `key: value` and `- item` lists only). Fields: name, minutes (default 90), phases (ordered list `name minutes-or-percent`, e.g. warm-up 10 / work 60 / demo 15 / retro 5), default_step_minutes, nudge_points (halfway, 2m-left), pairing rule, desk tools per phase, tone/guardrails text, teacher_steps (prepare, facilitate, review). Body = guidance for the planning agent.
- `tutor plan "GOAL" --minutes N [--people a,b] [--format F] [--agent A] [--title T] [--epic lern-psic] [--json]` prints a draft (common.py schema v1). With an agent (`--agent` → `$TUTOR_AGENT`): one-shot `bashy chat --agent A -m <prompts/plan.md filled> --json`, read `.output`, extract the JSON object, `validate_draft`; on any agent failure or invalid JSON fall back to the template split and set `"source": "template"` (else `"agent"`). Template split: phases scaled to --minutes, student steps round-robin owners over --people (empty owner when none), plus the format's teacher steps owned by `teacher`. Step minutes must sum to --minutes for student steps.
- `tutor apply DRAFT|- [--json]`: validate, `bashy sprint add TITLE --epic E --primary-goal GOAL --json` (read the number), `bashy person add` each distinct non-empty owner (ignore 'already exists'), `bashy todo add TITLE --sprint N --owner P --label <common.step_labels(...)> --note "done when: ..." --json` per step in order. Print {sprint, steps:[{index,id,title,owner}]}. Idempotency not required, but refuse a draft that fails validation BEFORE creating anything.
- Tests: no-agent template plan (sums, owners, roles); stub agent via `BASHY_BIN` pointing to a tiny fake script that answers `chat` with a canned envelope and execs real bashy otherwise — valid JSON accepted, garbage → template fallback; apply in TempBashy creates the card + labelled todos (check with `bashy todo list --json --label step-01`).
