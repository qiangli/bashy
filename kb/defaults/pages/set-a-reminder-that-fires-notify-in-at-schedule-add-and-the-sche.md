---
id: 01a21595-404b-71d8-a56e-cab1083fd62b
seq: 36
form: page
type: runbook
title: Set a reminder that fires (notify --in/--at, schedule add, and the schedule engine)
description: 'How to remind a person, agent or role at a time or after a delay, run a command on a schedule, and make sure the schedule engine is actually running so the reminder fires — WHEN a user asks for a reminder, alarm, follow-up or timed notification, or a reminder never arrived (keywords: reminder, remind, alarm, notify, --in, --at, schedule, tick, daemon, service enable schedule, calendar, inbox, overdue, never fired)'
tags:
    - reminder
    - schedule
    - notify
status: candidate
evidence: '2026-11-25, scratch schedule store: notify --in 1s --to a role seat stored a one-shot job, schedule tick fired it (fired 1 job, job shown [off]); schedule add --every stored a job; service status shows the opt-in unit state'
source:
    tool: claude-fable5.2
created: "2026-11-26T02:00:32Z"
---

A reminder is a job in the one bashy scheduler. It fires only while a schedule engine is running on the host, so check the engine first.

## Variables used below

```sh
WHO=<agent name, role, or role seat such as steward>
```

## 1. Check that the engine is running

```sh
bashy service status schedule     # ENABLED and STATE columns
bashy schedule list               # jobs with their next run time
```

- The `schedule` unit is opt-in. Where it is disabled, jobs are accepted and stored but nothing fires them, and `next` in `schedule list` stays in the past.
- bashy says so itself: the first line of `bashy schedule list` is `engine: running (pid N, overdue N)` or `engine: not running (overdue N)`, and `schedule add` and `notify --in/--at` print a one-line hint on stderr when the job is stored while no engine is running (silence: `BASHY_HINTS=off`).
- Turn it on for this host (the owner's decision): `bashy service enable schedule`.
- Without the unit: `bashy schedule tick` fires everything that is due once; `bashy schedule daemon` runs a foreground loop.

## 2. Set the reminder

Remind a person, agent or role (one subject line, no body):

```sh
bashy notify --in 15m --to "$WHO" "stand-up in 5 minutes: meet 3"
bashy notify --at 09:00 --to "$WHO" "review the overnight run"
```

- `--at HH:MM` is local clock time; `--in` takes a duration.
- Put the place to continue in the subject. A notification starts a conversation and never hosts one.
- If the recipient name is ambiguous (several live instances), address a role seat instead.

Run a command at a time or on a cadence:

```sh
bashy schedule add --at 17:30 --name wrap-up -- bashy notify --to "$WHO" "wrap up"
bashy schedule add --every 2h --name check -- <command>
bashy schedule add --cron "0 9 * * 1-5" --name weekday -- <command>
```

## 3. Where it lands

- The recipient reads it with `bashy inbox` (and on the board, `bashy mb`).
- To deliver outside bashy, subscribe a sink: `bashy bus subscribe --sink exec -- <command>` or `--sink tool-cmd TOOL:NAME`.
- Desktop pop-up: the agent-hub skill has a `desktop` function for macOS (`osascript`) and Linux (`notify-send`). There is no Windows desktop toast yet.

## 4. Review and remove

```sh
bashy schedule list
bashy schedule disable <id>
bashy schedule rm <id>
```

## Gotchas

- A reminder that never arrived: look at `next` in `bashy schedule list`. A past time means no engine was running; run `bashy schedule tick` and decide about the unit.
- A one-shot job shows `[off]` after it has fired; that is the record that it ran.
- Calendar apps do not run reminders themselves; wire them with the commands above.
