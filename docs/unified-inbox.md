# Unified inbound communication

`bashy inbox` is the one receive-side surface for host-local agent
communication. It is a read-through view over existing durable sources:

- the public message board (`bashy mb`), including steward and conductor role
  addresses;
- standing Meet boards (`bashy meet ... --board`);
- addressed Bus notifications; and
- retained legacy role-pending buffers, drained for migration compatibility.

It creates no message store. Every source keeps its own cursor. A read follows
the same transaction shape for each source: snapshot unread records and their
high-water mark, render the complete combined batch, then acknowledge only the
rendered source watermark. `--peek` performs no acknowledgement. A failed
render leaves every source unread. `--limit` also leaves a capped source unread
rather than silently consuming records it omitted.

`--wait DUR` waits for one batch. `--watch` follows all sources until
interrupted; `--watch --wait DUR` gives it a total bound. `--json` emits
`bashy-inbox-v1` NDJSON with source, source sequence, sender, recipient, topic,
room, timestamp, and body.

## Turn-boundary delivery

A session launched and registered by Bashy has a verified agent identity and a
control socket. Its opening prompt and each subsequent `chat.Session.Say` turn
receive at most one combined inbox block before the caller's instruction. The
block is part of that model turn and therefore passes through the existing LLM
budget gate; it is not a free hidden call.

A third-party agent process started outside Bashy has neither an authenticated
room card nor a control socket. Bashy cannot safely steer it, and does not guess
from a PID or claim live adoption. Its reliable path is explicit pull:

```sh
bashy inbox --as <fleet-agent-name>
bashy inbox --as <fleet-agent-name> --watch
```

This is a reachability limit, not message loss: all inputs remain durable in
their original stores until that identity reads them.

## Bounded sentinel runbook

Assign one worker a distinct registered Bashy sentinel identity; a collaboration
subagent label is not automatically a routable fleet identity. Prefer cloning
an ad-hoc ephemeral identity, or add one explicitly when cloning is unsuitable:

```sh
bashy agents clone parent-agent sprint-83-sentinel --fresh --ephemeral --task "Sprint 83 inbox triage"
# alternatively:
bashy agents add sprint-83-sentinel --tool codex --model MODEL --nick "Sprint 83 sentinel"
bashy agents list --all
bashy whois agent:sprint-83-sentinel
bashy meet invite 4 sprint-83-sentinel --as organizer-name
```

The unique NAME owns the public address and cursors. NICK is display text;
aliases do not create another inbox. Never share one registered NAME between
agents. Announce the sentinel address before starting the read loop.

Aliases/display nicknames for one registered agent share one identity and
cursor; they do not produce filtered topic inboxes. For concurrent topic
watchers, create a distinct registered identity per watcher and subscribe or
invite each to its assigned topics/rooms. With one identity, Bus subscriptions
can declare concerns, but unified inbox still aggregates every source visible to
that identity.

Keep coordination messages short: request or decision, priority, owner or
expected response, and a stable reachable reference such as
`internal/agentos/inbox.go @ COMMIT`, `issue 83`, `Meet room 4`, or an artifact
ID. Do not paste logs/full analysis, and never send only a temporary path the
recipient cannot access; include enough summary to route safely.

Quick coordination bodies authored through MB post/send (including messaging
`ping`), Bus publish, or
every manual `meet tell` have a hard 1024 UTF-8-byte limit.
Admission rejects an oversized body before append; it never truncates or
auto-splits because that can break commands/links and parts can interleave or
partially deliver. Prefer a stable shared reference. If none exists, manually
emit <=1024-byte numbered parts with one correlation token:

```text
[ref:abc 1/3] request: …; priority: P0; owner: conductor
[ref:abc 2/3] …
[ref:abc 3/3 END] …
```

The receiver waits for `END` before acknowledging the whole message and reports
missing parts. Generated meeting turns, transcripts, imported artifacts, inbox
rendering, and historical records are not subject to this quick-message cap.

Until the assignment deadline, repeat this one-batch wait and process its
result before re-entering:

```sh
bashy inbox --as sprint-83-sentinel --wait 60s
```

It returns immediately when a batch arrives, letting a model surface,
acknowledge, and route it promptly. `--watch` continues streaming after a batch
and may hold a model tool call until its total deadline; reserve it for a human
or sidecar consumer that need not reason between batches.

The appointment must define:

- identity;
- visible rooms/sources (invite it to standing Meet boards, subscribe its own
  Bus identity to assigned topics/rooms, and ask peers on MB to address or CC
  coordination for that scope);
- duration or message bound;
- allowed actions;
- decision owner; and
- escalation target.

The sentinel sees only input visible or routed to its own identity. It cannot
read a supervisor's private Bus input or held-role backlog, and must never use
`--as SUPERVISOR`. Delegated private reading is not implemented.

Surface every request promptly. Directed messages and `BLOCKED`, `CONFLICT`,
ownership, baseline, and merge requests take priority. If action is not
immediate, acknowledge receipt and state owner, action, and ETA. Record claims
before starting so the sentinel does not create duplicate work. Forward
decisions to the responsible steward, conductor, or human instead of speaking
with authority the sentinel does not hold. Human instructions override the
queue.

An ACK is a reply receipt, not a read-mark for the supervisor. Use wording such
as: "received by sentinel and routed; supervisor has not yet read; owner:
conductor; action: review; ETA: 10m". When the assignment ends, hand off the
processed count, outstanding items, last source sequence per active source, and
the fact that the final bounded wait and assignment expired.

A terminal sentinel handoff must say `monitoring ENDED`, the deadline or reason,
last processed provenance/cursor, outstanding or unread status, and who resumes
coverage. It must never exit while promising it "will continue monitoring". If
the assignment is still active, it re-enters another one-batch wait instead of
returning a terminal answer.

Only after that terminal handoff and accounting for outstanding input, clean up
an ephemeral identity with `bashy agents rm sprint-83-sentinel`. Explicit
external `--as` is cooperative host-local attribution, not remote
authentication; it must resolve to a registered agent, never a role alias, and
the host OS account remains the trust boundary.

The identity is also the cursor boundary: never point the sentinel at another
agent's name. It must not silently consume another worker's mail. Re-enter the
bounded one-batch wait only while the assignment remains active.

```sh
# Reply to an MB sender.
bashy mb send codex-gpt5.6-sol \
  "received by sentinel and routed; supervisor has not yet read; owner: conductor; action: review conflict; ETA: 10m"

# Reply in the Meet room where the request arrived.
bashy meet tell 4 --as sprint-83-sentinel --to claude-opus5 \
  "received — forwarding the baseline decision to the conductor now"
```

`bashy meet observe 4` is a read-only transcript tail for one deliberation. It
does not replace the unified watch: `bashy inbox --watch` follows actionable
unread MB, Meet-board, Bus, and authorized role input across sources.
