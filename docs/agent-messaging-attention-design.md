# Agent messaging + the attention problem — design of record

**Status:** design of record (2026-07-21), settled in a steward session. Brand-neutral
(bashy/coreutils mechanism; no dhnt-internal references). Extends the notification-bus
and room-mesh designs; this doc adds the part they left open — **how a running agent
learns it has mail without being told to look, and what that costs.**

## The goal (user's framing)

A Slack-like fabric for the agent ecosystem: any agent (worker, conductor, steward) can
**message any other**, open an **ad-hoc room** or a **1:1**, to (a) file issues, (b)
volunteer for work, (c) coordinate / chitchat. Discovery = membership; connection = send a
timeline event (the room model already seeded in `pkg/room`).

## The hard part: attention, not transport

Transport is the easy half and mostly exists (`bashy watch` subscribe side, `pkg/room`
timeline, `bashy chat sessions/timeline/steer`, `bashy foreman tell/interrupt`, `bashy kb`).
The missing half is **delivery of attention**: a channel nobody reads is a dead-letter box.

A running agent is always in one of three states, and **it can never reliably decide on its
own to go check a channel**:

- **Heads-down in a turn** — executing tool calls; no free cycle to poll, can't context-switch without a signal.
- **Between turns** — blocked on stdin, waiting for input.
- **Idle / no work** — done, sitting there.

Prompting the agent to "remember to check your inbox" is the **worst** option (it forgets, or
it's mid-task). **The trigger must come from the harness, not the agent's cognition.** Attention
management is a harness concern — consistent with the project thesis that the harness delivers
the capability while the agent stays focused on the task.

## The subscriber sidecar (the load-bearing new component)

Because the agent cannot watch a channel while busy, a **lightweight sidecar process holds the
subscription continuously on the agent's behalf** and maintains a **pending-buffer** (the delta
since the agent last read). The agent never subscribes or polls; it reads a pre-resolved buffer
the sidecar already computed **off the agent's critical path**. The sidecar also decides
*relevance* and *urgency* (which inject point to use), so the agent is never handed noise.

This is the coach model generalized: a cheap second process watches the bus/room + the agent's
event stream, and translates "message arrived at any time" into "delivered at a boundary the
agent can actually receive."

## The three inject points (urgency-tiered, harness-owned)

Map each agent state to the mechanism the harness controls at that point:

| Agent state | Inject mechanism | Urgency it serves | Today |
|---|---|---|---|
| **Between turns (turn boundary)** | Sidecar's pending-buffer injected into the *next prompt* (pull, cooperative). Works for **every** agent, even dumb non-steerable CLIs — stdin-wait is the universal seam. **This is the floor.** | FYI, durable, "here's what changed" | `foreman tell` queues to the boundary; weave injects `KB.md`/goal at workspace start |
| **Heads-down mid-turn** | Sidecar injects an **ESC interrupt** on the steer side-channel between tool calls (push, preemptive). Expensive — discards in-flight work — so **rare, direction-changers only**. | "Stop, the plan changed" | `foreman interrupt` / coach ESC, steerable harnesses only (ycode `--events`) |
| **Idle / no work** | A **periodic tick** polls queue+bus (or blocks on a subscription the transport wakes). The worker twin of the steward supervision tick. | "Is there volunteer work / a message?" | steward loop; `weave heartbeat` for workers |

Endgame (north star, not this build): **event-driven server mode** — the agent runs as a
service and the transport *wakes it* on a message (A2A `SubscribeToTask`), no polling at all.
Deferred: today's harnesses run a turn loop, and server mode isn't reached (the agent loop lives
in a server process that doesn't see the client's `--events`).

## Performance — what "check every turn" costs

Two regimes, and the sidecar keeps the common one free:

| | Empty inbox (common) | Non-empty inbox |
|---|---|---|
| **Compute/latency** | Local read of a flag the sidecar already resolved out-of-band — µs–ms, imperceptible vs a multi-second turn | Same tiny read + prefill of the injected delta |
| **Tokens** | **Zero** (nothing injected) | Delta tokens only |
| **Attention** | None | Injected text competes with the task |

Three disciplines bound the non-empty cost:

1. **Delta-only, not state.** The bus is change-notification; inject a short header ("2 new
   since last turn: X asked Y; steward flagged Z"), not channel history. Durable content stays
   in `kb` (pull); the inject is just the ping. Tens of tokens, not thousands.
2. **Tail-inject to preserve the prompt cache.** Splice the delta at the **end** of the context,
   after the stable system+tools+history prefix, so only the tail re-prefills. Injecting into the
   middle busts the KV/prompt cache and forces a full re-prefill — that is the one thing that would
   make per-turn injection genuinely expensive. Tail-inject keeps the marginal cost ≈ the delta
   tokens on a warm cache.
3. **Attention is the budget, not milliseconds.** The documented failure mode is steering collapse
   under a flooded flat context. So the sidecar gates by relevance and routes by urgency: FYI →
   cheap turn-boundary; direction-changer → ESC (rare); chitchat → surfaces only when idle, never
   interrupts.

**Bottom line:** per-turn checking is ~free on an empty inbox (sidecar absorbs the poll; zero
tokens; tail read) and bounded-small otherwise (delta header + tail-inject + warm cache). It moves
none of the agentic-cost metric (wall × calls × tokens) in the empty case. This must be *measured*,
not asserted — see the acceptance gate.

## Phased build

- **P0 — `bashy notify` publish verb.** Complete the bus: `watch`/subscribe already ships; add the
  publish side. Topic-based (`--topic`), principal-tagged (report/author split — a publish carries
  who sent it; a redirect is authored+authorized), 1:1 (`--to <session|role>`) and room
  (`--room <id>`) addressing. Local bus store; cloudbox/matrix relay is a later cross-host phase.
  Buildable standalone, ≤3pt.
- **P1 — subscriber sidecar.** A process (or goroutine in the launcher) that holds an agent's
  subscriptions, maintains the delta pending-buffer, and exposes a local read + a relevance/urgency
  tag. Off the agent's critical path.
- **P2 — the three inject points.** Wire the pending-buffer into (1) turn-boundary inject (the
  universal floor: delta-only header, **tail-injected**), (2) ESC interrupt for direction-changers
  (steerable harnesses), (3) idle-tick poll (volunteer-for-work). Carries the perf acceptance gate.
- **P3 — role auto-subscribe.** The launcher auto-joins each role (steward/conductor/worker) to its
  role topic + a direct 1:1 address, so no role-specific code and **no agent is ever told to check.**

## Acceptance gates

- P0: `go build ./... && go test ./...`; a publish is readable by an existing `watch` subscriber; a
  publish without a principal is rejected (report/author invariant).
- P2 (**the load-bearing gate**): a `perfbench` A/B — an **empty-check turn vs a baseline turn** —
  shows **no token growth and a preserved prompt cache** (tail-inject verified: prefix cache hit
  unchanged). A non-empty inject stays within a capped delta-token budget. Never merge P2 on the
  claim that it's cheap — measure it.
- P3: launching any role through the governed launcher auto-subscribes it; a message to a role
  surfaces at that role's next turn boundary with no code specific to the role.

## Non-goals / guards

- No agent self-polling in-turn (harness owns attention).
- ESC interrupts stay rare (they discard work) — FYI never interrupts.
- Chitchat never preempts a working agent — idle-surface only.
- Cross-host federation (cloudbox/matrix relay) is a later phase; P0–P3 are local-first.
