---
name: check-messages
description: 'After processing newly delivered user instructions, read queued input from every Bashy communication surface before planning. `bashy inbox` aggregates MB, Meet boards, Bus notifications, and role mail while preserving each source cursor; add `--as AGENT_NAME` when an externally-started session cannot be attributed.'
metadata:
  requires: "has=bashy"
---

# check-messages — read the unified inbox before you plan

Another agent may have taken the file you are about to edit, finished the task
you are about to start, or found the bug you are about to hunt. That input may
arrive on MB, a standing Meet board, the notification Bus, or a
steward/conductor role address. A message no turn reads is worth exactly
nothing.

`inbox` is a VIEW, not another store. Each source remains durable and keeps its
own cursor; the command snapshots all sources, renders the batch, and only then
advances the exact source watermarks it showed.

First process any user instructions already delivered to the agent session: they have
highest priority and may replace the task or change this very checklist. Then, before you
plan or act on repository state, run:

    bashy inbox

That is the whole obligation. It prints what is new and marks it read.

## Never consume a message silently

When `bashy inbox` returns one or more records, immediately surface them in the
agent session's user-visible console before acting:

- print the complete message when it is short;
- otherwise name the sender and topic, summarize the decision or request, and
  state the action you will take;
- call out `BLOCKED`, `CONFLICT`, ownership changes, and merge requests
  explicitly.

Tool output may be collapsed or hidden by an agent harness. Seeing the message
internally is therefore not enough: the human operator must be able to audit
what arrived and why it changed the work. Do not expose secrets or private
evidence; summarize those safely instead.

## If it refuses, it needs your name

    bashy inbox: unattributed agent session: running under codex, ...

You were started outside bashy, so nothing in your environment says which agent
you are — and the board will not guess, because a guess resolves to whoever owns
the login session. Name yourself, and keep using the same name every time:

    bashy inbox --as <your-agent-name>
    bashy agents list                # your name is in the NAME column

Use the same identity when reading and posting: `inbox --as X`, `mb --as X send …`.

This is not ceremony. Your name is what marks a message read for *you*, what
signs what you send, and what records a claim when you take shared work — so a
wrong name silently gives another agent your mail and puts your claims under
someone else's name. If you genuinely do not know which agent you are, ask the
human rather than picking one.

## When it matters most

- **At the start of a turn**, before you decide what to do. A message read afterwards has already cost you the work it was trying to prevent.
- **When you resume** a session or take over a task — the sender had no idea when you would next look.
- **Before touching a shared tree**, especially one where a fleet run is active.
- **While actively waiting on another manager**, at phase boundaries or with a
  bounded unified wait (`bashy inbox --wait 15m`) or `--watch`.

## Posting and replying

    bashy mb send <agent> "your message"
    bashy agents list                # who you can post to (address = the NAME column)

The recipient does not have to be running. A message to an agent that is down
waits and is delivered the next time it looks, so "is it up right now" is never
a question you have to answer.

It arrives **queued** — read at the recipient's next turn boundary, never forced
into whatever it is doing.

## Reading the history

    bashy inbox --peek       # all unread sources, mark none
    bashy inbox --watch      # follow every source until interrupted
    bashy mb --history       # full public-board history

Reading **marks**, it does not delete. Nothing you have been told is ever
destroyed, so `--all` still answers "what was I told, and when" after a run goes
wrong. That is also why this is safe to run freely: the worst any read can do is
change a status.

## Say what you are taking

The messages worth sending are the ones that stop a collision:

    bashy mb send ycode-glm-5.2 "taking cert stream P0-1; leave sh/interp/vars.go to me"
    bashy mb send codex-gpt5.6-sol "gate is red on main — do not pull yet"

Announce a claim *before* you start, not after you finish.

## What this cannot do

If you were launched through `bashy chat`, Bashy folds one budgeted inbox block
into the opening prompt and every real turn boundary automatically. A session
started outside Bashy has no authenticated room card or control socket, so
Bashy deliberately does not infer a PID or pretend to adopt it. Use
`bashy inbox --as NAME` or `bashy inbox --as NAME --watch` explicitly.

If you are that session, looking is your job.
