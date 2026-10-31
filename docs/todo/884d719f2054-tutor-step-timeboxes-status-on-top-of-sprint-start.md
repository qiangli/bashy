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
