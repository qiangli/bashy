# Tutor

Start a session like a coding sprint: an editable plan becomes a sprint card,
with timeboxed steps owned by people and an owner's agent managing the session.
The same tool supports group sessions, meetings or solo work. Python 3 stdlib and bashy
are sufficient for the planning and run verbs; the display uses Go.

From the repository root, register the single Bash# body as command, skill and
app (port 18093):

```sh
bash examples/tutor/register.sh
bashy tutor --help
bashy commands show tutor --json
bashy skill show tutor
```

Registration writes your selected bashy catalogs. For an isolated rehearsal,
use the verification command below instead; it creates temporary catalogs,
including a separate app registry, and removes them afterward.

## Session

```sh
bashy tutor plan 'Build and demonstrate a small improvement' \
  --minutes 90 --people ana,ben --format examples/tutor/formats/session.md \
  --title 'Session' --json > draft.json
# Review/edit titles, owners, timeboxes and observable done_when evidence.
bashy tutor apply draft.json --json
# Replace N with the returned sprint number; AGENT is the owner's agent.
bashy sprint start N --owner AGENT --for 90m \
  --instruction "run today's session with the tutor skill"
bashy tutor start N --json
bashy tutor status N --line
bashy tutor done N STEP -m 'Working result demonstrated' --json
bashy tutor next N --json
bashy tutor extend N STEP --by 5m --json
bashy tutor nudge N --person ana --json
```

STEP is the todo ID or its original numbered step label. Participant progress
counts only participant steps; owner steps remain in the JSON `steps[]`.
People never close sprint stories. `tutor done` records a done event in
`$XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl` and adds the `step-done` label.
The session manager (the owner's agent) accepts each finished step at the end:

```sh
bashy sprint accept N ID -m 'Evidence reviewed by the owner'
```

The session clock is the sprint's own clock. Tutor reads it from the card
and falls back to the run log written by `tutor start --for`.
`tutor start N --owner AGENT --for 90m` can start a session directly.
With no agent, planning uses the format template and nudges use fallback
questions. Starting a new session requires an explicit manager and otherwise
returns a clean error. Offline `next`, `done` and `status` remain available;
they do not invent a running session clock.

## Meeting and solo work

These examples use the meeting and solo formats. Copy and edit a format for
your own phases.

```sh
bashy tutor plan 'Agree on a release decision' --minutes 30 \
  --people ana,ben --format meeting --json > meeting.json
bashy tutor plan 'Build and demonstrate one small improvement' --minutes 25 \
  --people me --format solo --json > solo.json
```

Review either draft, then `apply`, start the sprint with a manager, and use
the same run verbs. Set `--agent AGENT` or `TUTOR_AGENT` to request an AI
draft; an unavailable or invalid agent response falls back to the template.

## Make your own format — Software Shaping

```sh
cp examples/tutor/formats/session.md my-meeting.md
# Edit my-meeting.md, then:
bashy tutor plan 'Decide the next experiment' --format ./my-meeting.md \
  --minutes 30 --people ana,ben --json > draft.json
```

The format is the software-shaping surface: change phases and their minute
shares, default step minutes, pairing, desk tools, tone, nudge points and
owner steps. For a meeting, try explore / decide / commit phases; for solo
work, try focus / demonstrate / reflect. Front matter supports simple
`key: value` fields and `- item` lists. The offline planner scales phases and
splits participant work into timeboxes; the prose, pairing and tone guide the
planning agent. Review the resulting JSON before applying it.

`roles:` is an ordered list of label words: the first is the owner role, the
rest are participant roles. `shared` is reserved for editions; `owner` may
only name the first role. If absent it defaults to `owner`, `participant`.
`owner_steps:` declares preparation/facilitation/review work. `audience:` and
`tone:` supply the nudge wording; an empty audience adds no audience sentence.
The default is `formats/session.md`; the other files are example formats.
Session-specific agendas belong in the owner's own files, outside this repository.

Apply snapshots `format`, ordered `roles`, `audience` and `tone` in a `format`
event in `$XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl`. Status exposes `format`
and `roles: {owner, participants}`. Later edits to a format do not change an
applied session. Progress and next/start use participant-role steps; owner
steps remain in `steps[]` and can be marked done explicitly. For a single-role
format, that role performs the session work and counts toward progress.

## Private and shared editions

Prepare a content directory before session with `bashy tutor prep DIR --goal
"GOAL" --minutes 90 --json`, or copy an existing one with
`bashy tutor prep DIR --from PATH --json`. Edit the format, content and
`DIR/draft.json`, then apply that draft. Directory copies can be kept in git
as owner/year variants.

Keep one source per content item, with answers, rubric and hint ladder inside
`~~~private` blocks. Render the owner edition for preparation and the
shared edition for sharing:

```sh
bashy tutor render examples/tutor/content/example-step.md --edition owner
bashy tutor render examples/tutor/content/example-step.md --edition shared
```

`~~~private` is owner-only. `~~~private ROLE...` also grants the named roles
access to that block. `--edition owner` or the format's owner role sees all;
`--edition shared` sees no private blocks; `--edition ROLE` sees blocks granted
to that role. Answers in front matter remain owner-only. Rendering resolves
the format from the content's `format:` field, a neighboring `format.md`
(in the content directory or its parent), or the default session format.
Unknown editions are rejected. `--format form` emits the question form. Hints should ask a question
or suggest the next small step; they should never supply the participant's work.

## Skill

```sh
TUTOR_SPRINT=N bashy skill run tutor --json
```

`tutor status N --line` prints one plain line, so any tool can carry it (for
example a chat app's topic); tutor itself knows nothing about chat.

## Isolated rehearsal and gate

```sh
cd examples/tutor
bashy dag dag.md dry-run
python3 -m unittest discover -s tests -t . -v
```

`verify-duality.py` runs registration and plan → apply → start → done/next →
status in temporary BASHY_HOME, coordination, room, XDG and app directories
and a temporary git repo. It compares direct script and registered command
JSON, checks skill projection and attestation, and rejects a missing sprint.

The five rehearsal steps are frame the goal (15m), explore options (20m),
build an example (25m), demonstrate the result (20m), and reflect (10m),
plus three owner steps. The no-agent run verifies start's explicit-manager
error and continues offline. The stub run supplies a deterministic planning
response and acknowledges sprint start, checking the 90-minute fallback clock.
It does not prove a real agent launch, LAN/phone access or chat delivery.
No live board, accounts or server are changed.
