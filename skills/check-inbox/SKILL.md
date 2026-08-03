---
name: check-inbox
description: 'Read messages other agents and humans sent you, at the START of every turn, before planning. Use when you begin a turn, resume a session, or pick up work in a repo where other agents are active. `bashy inbox` shows what is new and marks it read; `bashy im <agent> "..."` sends. Costs one command and prevents duplicated or conflicting work.'
metadata:
  requires: "has=bashy"
---

# check-inbox — read your mail before you plan

Another agent may have taken the file you are about to edit, finished the task
you are about to start, or found the bug you are about to hunt. It has no way to
tell you except by leaving a message — and a message you never read is worth
exactly nothing.

**Run this first, before you plan the turn:**

    bashy inbox

That is the whole obligation. It prints what is new and marks it read.

## When it matters most

- **At the start of a turn**, before you decide what to do. A message read afterwards has already cost you the work it was trying to prevent.
- **When you resume** a session or take over a task — the sender had no idea when you would next look.
- **Before touching a shared tree**, especially one where a fleet run is active.

## Replying and sending

    bashy im <agent> "your message"
    bashy agents list                # who you can write to (address = the NAME column)

The recipient does not have to be running. A message to an agent that is down
waits and is delivered the next time it looks, so "is it up right now" is never
a question you have to answer.

It arrives **queued** — read at the recipient's next turn boundary, never forced
into whatever it is doing.

## Reading the history

    bashy inbox --peek     # read without marking anything read
    bashy inbox --all      # every message ever received, read or not

Reading **marks**, it does not delete. Nothing you have been told is ever
destroyed, so `--all` still answers "what was I told, and when" after a run goes
wrong. That is also why this is safe to run freely: the worst any read can do is
change a status.

## Say what you are taking

The messages worth sending are the ones that stop a collision:

    bashy im ycode-glm-5.2 "taking cert stream P0-1; leave sh/interp/vars.go to me"
    bashy im codex-gpt5.6-sol "gate is red on main — do not pull yet"

Announce a claim *before* you start, not after you finish.

## What this cannot do

This is an instruction, and an instruction is a soft guarantee: nothing enforces
that you ran it. If you were launched through `bashy chat`, you do not need this
skill at all — your mail is folded into your opening prompt and prepended at
every turn boundary automatically. This file exists for the case that cannot be
automated: a session started outside bashy, where the only way to see a message
is to look.

If you are that session, looking is your job.
