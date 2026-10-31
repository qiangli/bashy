# Tutor

Start a class like a coding sprint: an editable plan becomes a sprint card,
with timeboxed steps owned by people and a teacher's agent managing the session.
The same tool supports a class, meeting or solo work. Python 3 stdlib and bashy
are sufficient for the planning and run verbs; the projector uses Go.

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

## Class

```sh
bashy tutor plan 'Week 2: Klatch sign-up, groups and after-class use' \
  --minutes 90 --people ana,ben --format examples/tutor/formats/class.md \
  --title 'Nov 1 · Week 2' --json > draft.json
# Review/edit titles, owners, timeboxes and observable done_when evidence.
bashy tutor apply draft.json --json
# Replace N with the returned sprint number; AGENT is the teacher's agent.
bashy sprint start N --owner AGENT --for 90m \
  --instruction "run today's session with the tutor skill"
bashy tutor start N --json
bashy tutor status N --line
bashy tutor done N STEP -m 'Greeting visible in the group' --json
bashy tutor next N --json
bashy tutor extend N STEP --by 5m --json
bashy tutor nudge N --person ana --json
```

STEP is the todo ID or its original numbered step label. Student progress
counts only student steps; teacher steps remain in the JSON `steps[]`.
People never close sprint stories. `tutor done` records a done event in
`$XDG_DATA_HOME/bashy/tutor/<sprint>.jsonl` and adds the `step-done` label.
The class manager (the teacher's agent) accepts each finished step at the end:

```sh
bashy sprint accept N ID -m 'Evidence reviewed by the teacher'
```

The session clock is the sprint's own clock. Tutor reads it from the card
and falls back to the run log written by `tutor start --for`.
`tutor start N --owner AGENT --for 90m` can start a session directly.
With no agent, planning uses the format template and nudges use fallback
questions. Starting a new session requires an explicit manager and otherwise
returns a clean error. Offline `next`, `done` and `status` remain available;
they do not invent a running session clock.

## Meeting and solo work

These examples reuse the class phases at a different duration. For your own
phases, copy and edit the format as described below.

```sh
bashy tutor plan 'Agree on a release decision' --minutes 30 \
  --people ana,ben --json > meeting.json
bashy tutor plan 'Build and demonstrate one small improvement' --minutes 25 \
  --people me --json > solo.json
```

Review either draft, then `apply`, start the sprint with a manager, and use
the same run verbs. Set `--agent AGENT` or `TUTOR_AGENT` to request an AI
draft; an unavailable or invalid agent response falls back to the template.

## Make your own format — Software Shaping

```sh
cp examples/tutor/formats/class.md my-meeting.md
# Edit my-meeting.md, then:
bashy tutor plan 'Decide the next experiment' --format ./my-meeting.md \
  --minutes 30 --people ana,ben --json > draft.json
```

The format is the software-shaping surface: change phases and their minute
shares, default step minutes, pairing, desk tools, tone, nudge points and
teacher steps. For a meeting, try explore / decide / commit phases; for solo
work, try focus / demonstrate / reflect. Front matter supports simple
`key: value` fields and `- item` lists. The offline planner scales phases and
splits student work into timeboxes; the prose, pairing and tone guide the
planning agent. Review the resulting JSON before applying it.

## Teacher and student editions

Prepare a content directory before class with `bashy tutor prep DIR --goal
"GOAL" --minutes 90 --json`, or copy an existing one with
`bashy tutor prep DIR --from PATH --json`. Edit the format, content and
`DIR/draft.json`, then apply that draft. Directory copies can be kept in git
as teacher/year variants.

Keep one source per content item, with answers, rubric and hint ladder inside
`~~~teacher` blocks. Render the teacher edition for preparation and the
student edition for sharing:

```sh
bashy tutor render examples/tutor/content/example-step.md --edition teacher
bashy tutor render examples/tutor/content/example-step.md --edition student
```

Teacher blocks are excluded from student output. Hints should ask a question
or suggest the next small step; they should never supply the student's work.

## Chat composition and skill

```sh
examples/tutor/chat-status.sh N CONFIG.json GROUP
# One update only:
examples/tutor/chat-status.sh N CONFIG.json GROUP --once
TUTOR_SPRINT=N bashy skill run tutor --json
```

The shell helper pipes `tutor status --line` to
`bashy klatch topic CONFIG.json GROUP -` and posts a milestone on step
change. It needs an existing Klatch bot configuration and group. Tutor does
not import Klatch or form code. The skill attests a read of the explicitly
selected sprint; it does not start a session.

## Isolated Nov 1 rehearsal and gate

```sh
cd examples/tutor
bashy dag dag.md dry-run
python3 -m unittest discover -s tests -t . -v
```

`verify-duality.py` runs registration and plan → apply → start → done/next →
status in temporary BASHY_HOME, coordination, room, XDG and app directories
and a temporary git repo. It compares direct script and registered command
JSON, checks skill projection and attestation, and rejects a missing sprint.

The five rehearsal steps are sign in (15m), join the group (20m), DM and
@mention (25m), demonstrate an after-class question (20m), and reflect (10m),
plus three teacher steps. These are rehearsal timings for the Week 2 theme.
The no-agent run verifies start's explicit-manager error and continues offline.
The stub run supplies a deterministic planning response and acknowledges
sprint start, checking the 90-minute fallback clock. It does not prove a real
agent launch, class sign-in, LAN/phone access or chat delivery.

No live board, class server, Sprint 510 or Sprint 528 is changed. The manager
links this rehearsal evidence to Sprint 528; the actual class outcome is
recorded after the session.
