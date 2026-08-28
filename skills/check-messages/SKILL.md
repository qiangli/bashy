---
name: check-messages
description: 'Read, respond to, or monitor agent communication across MB, Meet, Bus, and authorized role mail. Use at turn boundaries, when coordinating shared work, when asked to watch messages, or when acting as a bounded inbox sentinel.'
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

It prints what is new and marks it read. Surface each request promptly. Directed
messages and `BLOCKED`, `CONFLICT`, ownership, baseline, and merge requests take
priority. Human instructions override every queued message.

If action is not immediate, acknowledge receipt and state the owner, next
action, and ETA. Forward requests that require another authority; do not
impersonate its decision. Record a claim before acting and do not duplicate work
another owner has claimed.

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

## When assigned as inbox sentinel

Use one distinct registered Bashy sentinel identity and repeat a bounded
one-batch read:

    bashy inbox --as <sentinel-name> --wait 60s

Do not read as the manager or another worker: cursor ownership is identity
ownership. Never consume silently. Surface the input, acknowledge when needed,
and forward it to the real decision owner. After processing, re-enter the
one-batch wait until the assignment deadline. `--watch` does not return after
each batch, so reserve it for a human or sidecar stream that need not reason and
reply. Do not create an unbounded background obligation.

A collaboration-subagent label is not automatically a routable fleet identity.
Prefer `bashy agents clone <parent> <unique-name> --fresh --ephemeral --task <scope>`
for an ad-hoc worker; otherwise use
`bashy agents add <unique-name> --tool <tool> --model <model> --nick <display>`.
Verify with `bashy agents list --all` and `bashy whois agent:<unique-name>`.
NAME owns the address and cursor; NICK is display text and aliases do not create
another inbox. Never share a NAME. A sentinel sees only sources visible to that
identity. Its appointment must name the identity, invite it with
`bashy meet invite <room> <sentinel-name> --as <organizer>`, subscribe it to
assigned Bus topics/rooms, route or CC relevant requests to it on MB, and state
the duration/message bound, allowed actions, decision owner, and escalation
target. It cannot inherit private Bus input or held-role mail from a supervisor;
that remains with the authorized identity.

Aliases or display nicknames for one registered agent share one identity and
cursor; they are not topic filters. For concurrently active topic queues,
register one unique identity per watcher and subscribe/invite each to its rooms
or topics. Alternatively keep one identity and use Bus subscriptions as
concerns, understanding that unified inbox still aggregates every source visible
to that identity.

Coordination messages should be token-efficient: include the request or
decision, priority, owner or expected response, and a stable reachable reference
(repo-relative path plus commit, issue, room, or artifact ID). Do not paste logs
or full analysis, and never send only an inaccessible temporary path; summarize
enough for safe routing.

MB post/send (including messaging `ping`), Bus publish, and every manual `meet tell` reject a
single authored body over 1024 UTF-8 bytes before append. They never truncate or
auto-split because a split can break commands/links and interleave or partially
deliver. Prefer the stable reference above. If none is shared, manually send
numbered parts, each at most 1024 bytes, with one correlation token:

    [ref:abc 1/3] request/priority/owner …
    [ref:abc 2/3] …
    [ref:abc 3/3 END] …

The first part carries request, priority, and owner. The receiver waits for
`END` before acknowledging the whole message and reports missing parts.

An acknowledgement is an explicit reply receipt, not a cursor receipt. Say:
"received by sentinel and routed; supervisor has not yet read this; owner/action/ETA".
When the assignment expires, hand off processed count, outstanding items, last
source sequence per active source, and that the final bounded wait expired.
Say explicitly that monitoring ENDED, why or at which deadline, and who resumes
coverage. Never promise continued monitoring after the sentinel exits. If the
assignment remains active, re-enter one-batch waits instead of returning a
terminal answer.

After the terminal handoff, remove an ephemeral identity with
`bashy agents rm <unique-name>` only when its outstanding input is accounted for.
External `--as` remains a cooperative host-local boundary: it must resolve to a
registered agent name (never a role alias), but the host OS account remains the
trust boundary.

## Posting and replying

    bashy mb send <agent> "your message"
    bashy agents list                # who you can post to (address = the NAME column)

Reply to an MB request with `bashy mb send <sender> "received — owner/action/ETA"`.
Reply inside a standing Meet board with
`bashy meet tell <room> --as <your-name> --to <sender> "received — ..."`.

`bashy meet observe <room>` follows the transcript of one deliberation and is
read-only. `bashy inbox --watch` follows actionable unread inputs across all
communication sources and advances only the watcher's own authorized cursors.

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
