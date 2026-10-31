---
id: a9887361ae5b
kind: feature
title: klatch post/topic verbs + tutor status line in the class chat (composed)
seq: 3007
status: done
priority: p0
labels:
    - lern-psic
    - tutor
    - klatch
created: 2026-10-31T01:05:44.258613Z
weave: 394
assignee: claude-opus5.5
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
closed: 2026-10-31T02:04:00.108702Z
closed_by: s547-manager
---

Operator 2026-10-30: chat is the class surface this weekend (Sprint 528). Unix composition, no coupling: (1) bashy klatch gains generic verbs 'post GROUP TEXT' and 'topic GROUP TEXT' (stdin '-'), usable by anything; (2) tutor prints 'status --line'; (3) glue = a pipe/loop in the tutor skill or a dag step, e.g. bashy sprint watch N | while read _; do bashy tutor status N --line | bashy klatch topic class -; done, plus milestone posts. tutor never imports klatch and klatch never knows tutor. Bot account via bashy klatch provision. Verify the header shows on web + phone; if not visible enough, file a Klatch status-panel UI follow-up. Tests: fake Databag server (pattern examples/klatch/test_provision.py) + one live check on the class server before Nov 1.


## Brief (manager s547-manager, 2026-10-30)
Common rules for every S547 worker:
- Skeleton is on bashy main (97a7f240): `examples/tutor/tutor.bsh` dispatches each verb to ONE module per story. Edit ONLY the files listed under "Your files"; if you truly need a change elsewhere (tutor.bsh, lib/common.py) keep it minimal and say so in your submit message.
- Contract: `examples/tutor/lib/common.py` (draft schema v1, step labels `step-NN` `timebox-Nm` `role-student|role-teacher` `phase-WORD` — bashy labels cannot contain ':' so the spec's `timebox:20m` is `timebox-20m`; run log `$XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl`; `bashy(...)` runner wrapper honouring BASHY_BIN).
- Python 3 stdlib only (no pip), Go stdlib only for server/. tutor never imports or calls klatch/form code; composition is by pipes/JSON at the shell.
- Tests: unittest under `examples/tutor/tests/test_<area>.py`, subclass `tests.helpers.TempBashy` (temp BASHY_HOME, coord, room, XDG and git repo; cwd = temp repo). Run `cd examples/tutor && python3 -m unittest discover -s tests -t . -v`. Never touch the live board, the class Klatch server/data, or Sprints 510/528. Red first, then green.
- Every verb accepts `--json`. No agent configured must still work (no crash, sensible fallback).
- Speed first: MVP, focused test, no extra features. Commit with the Sprint/Story/Story-ID trailers of THIS story.

Your files: `examples/klatch/provision.py` (add verbs), `examples/klatch/klatch.bsh` (dispatch + skill text), `examples/klatch/test_provision.py` (tests), `examples/klatch/README.md`, and the glue `examples/tutor/chat-status.sh`.
- `bashy klatch post CONFIG.json GROUP TEXT|-` and `bashy klatch topic CONFIG.json GROUP TEXT|-` (stdin when `-`): generic, know nothing about tutor. Act as the config's bot account (e.g. a `bot` handle in the config; password via vault ref like the other accounts) — post a message to the group's channel / set the channel subject (Databag: channel `subject` data; see how provision.py creates groups and the Databag API). Output JSON {group, ok}. Keep klatch.bsh's existing setup/provision/status behaviour unchanged.
- `examples/tutor/chat-status.sh SPRINT CONFIG GROUP [--every 30]`: loop `bashy tutor status N --line | bashy klatch topic CONFIG GROUP -` only when the line changes, and `post` a milestone message when the current step changes. Pure shell composition.
- Tests: extend the fake Databag server pattern in test_provision.py to assert post and topic requests (auth as bot, right channel, text from stdin). Do NOT contact the real class server — the live check needs operator go-ahead and is done by the manager later. Note in your submit whether a channel subject is visible in Klatch web/phone headers per the code you read.
