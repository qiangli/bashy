---
id: a9e5f4c88cd9
kind: refactor
title: 'tutor/klatch/invite: make everything general — no class, roster, host or schedule baked in'
seq: 3023
status: todo
priority: p0
labels:
    - tutor
    - klatch
created: 2026-10-31T04:13:55.993725Z
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

Operator 2026-10-30: 'all the chat/tutor/invite should be general and should have nothing to do with a class nor with a fixed setup.' lern/psic is only a test cohort. Rework of accepted stories (operator requirement), no new features.
Found on bashy main (git grep class|teacher|student|lern|week 2 over examples/tutor + examples/klatch):
- roles hard-coded: lib/common.py accepts only role student|teacher; progress counts 'student steps'. → roles are words declared by the FORMAT (e.g. class: teacher/student; meeting: facilitator/participant; solo: self); code treats them generically (owner/host role vs participant roles).
- editions hard-coded: ~~~teacher blocks, --edition teacher|student. → generic private/shared: '~~~private [ROLE...]' blocks; --edition <role> or --edition owner|shared; format maps role names. Keep backward nothing (language still in design, no compat).
- nudge prompt hard-codes 'students aged 13 and up' → audience/tone come from the format.
- default format formats/class.md → generic default (formats/session.md); class.md, meeting.md, solo.md become example formats.
- dag.md + README: 'Nov 1 · Week 2' rehearsal and Klatch theme → generic example; class-specific agendas live in the owner's own files (lern: private psic2027 repo), never in bashy.
- server pages: Projector/Teacher/student labels → host/participant wording from the format.
- klatch README 'classbot' → generic bot; invite count = --count; handles/roster from the owner's config; serve on the owner's own machine (see klatch redesign mail).
Acceptance: git grep -i -E 'lern|psic|week ?2|nov 1|classbot|novidesign|alien' examples/tutor examples/klatch = 0 hits outside formats/examples; class/teacher/student appear only in the example class format + its tests; all tests green; a meeting-format and a solo-format plan/run/render test pass.

## Brief (manager s547-manager, 2026-10-30 ~21:30 PDT) — ONE worker owns the whole refactor
Base: bashy main (examples/tutor + examples/klatch as merged in S547). No backward compatibility (language still in design): rename freely, update every test.

Format contract (front-matter in formats/*.md; parsed by lib/plan.py's stdlib parser):
- `roles:` ordered list of words; the FIRST is the owner role (runs the session, sees private content), the rest are participant roles. Defaults if absent: `owner`, `participant`.
- `owner_steps:` replaces `teacher_steps` (prepare/facilitate/review style steps owned by the owner role).
- `audience:` free text (e.g. "students aged 13 and up") — the ONLY source of age/audience wording; nudge prompt reads it; empty → no audience sentence.
- `tone:` stays.
- Labels: `role-<word>` from the format (word rules: bashy label words). Progress (`current.index/total`, `--line`) counts participant-role steps only; owner steps stay in `steps[]`.
- `tutor status --json` gains `"roles": {"owner": "<word>", "participants": ["<word>", ...]}` and `"format": "<name>"` (store the format name + roles in the run log or as a card label at apply time, e.g. label `format-<name>`; pick one and document it).

Editions (lib/render.py): owner-only content in `~~~private` blocks; `~~~private ROLE...` also shows to the named participant roles. `tutor render ITEM --edition ROLE` renders what that role sees; the owner role sees everything; `--edition shared` = participant view with no role-specific private blocks. `--format form` unchanged in spirit.

Formats: default becomes `formats/session.md` (roles owner/participant, generic wording). `formats/class.md`, `formats/meeting.md` (facilitator/participant), `formats/solo.md` (single role `self`, no owner steps) are EXAMPLE formats. class/teacher/student may appear only in formats/class.md, its content example and its tests.

Server pages (server/*.go, pages.go, browser-test.mjs): page titles/labels from status JSON `roles` (owner page = capitalized owner role, participant page = participant role; projector stays "Projector"→ rename to neutral "Display"). Fallback when absent: "Host"/"Participant".

Docs/skill: tutor.bsh ~~~skill block, README.md, dag.md → generic (session/meeting/solo/class as examples); the rehearsal DAG uses the generic default format; no dates, cohorts, schedules, hosts. Class-specific agendas belong in the owner's own files — say so once in the README.

Klatch: README/klatch.bsh skill: `classbot` → generic `bot`; `invite CONFIG --count N [--ref REF]` (count is a required flag, owner mints more any time; update provision.py main + tests); chat-status.sh wording generic.

Acceptance (run exactly): `git grep -n -i -E 'lern|psic|week ?2|nov 1|classbot|novidesign|alien' examples/tutor examples/klatch` → 0 hits; `git grep -n -i -E 'teacher|student|class\b' examples/tutor examples/klatch` → hits only in formats/class.md, content examples for class, and tests that use the class format; full `cd examples/tutor && python3 -m unittest discover -s tests -t . -v`, `cd server && GOWORK=off go test -race ./...`, `cd examples/klatch && python3 -m unittest test_provision` all green; new tests: meeting-format plan→apply→next→status --line, solo-format plan/run, render `--edition` with `~~~private` and `~~~private ROLE`. Commit with this story's trailers; submit with the grep outputs.
