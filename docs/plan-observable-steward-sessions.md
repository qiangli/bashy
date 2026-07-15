# Plan: the room as the universal substrate (the 1:1 is the base case)

*Design, 2026-07-15. Started as "make steward sessions observable," then a sharper framing
collapsed it: **a human talking to a steward is already a meeting — a one-on-one meeting.**
So there is no special "steward session" to make observable. There is one primitive, the
**room**, it is fractal, and the only anomaly is that today's human↔agent channel is a
private pipe instead of a room.*

## The insight

A steward session (one agent orchestrating a fleet to a verified goal) and a meeting
(`bashy meet`) are not analogous — they are the **same thing at different sizes**. The
minimum meeting is **1 human + 1 agent**. That is the conversation a user has with a steward.
Everything larger is the same primitive with more participants:

```
root room:           human ── steward                     (the 1:1 — a meeting)
  └─ the steward adds participants / opens sub-rooms:
       sub-room:      steward ── codex   (OTLP fix)
       sub-room:      steward ── ycode   (MCP P3-P5)
       sub-room:      steward ── agy     (otel ui)
```

`observe` / `tell` / `say` apply at **every node, uniformly**. A second human `observe`s the
root room to watch the steward think; drills into a sub-room to watch codex; `tell`s to
interject at either level. "Leave the steward running in the background" is just: *the human
detaches from the room; the agents keep meeting in it.*

**The room is symmetric — it is the place where everyone meets.** A human joins a room the
same way an agent does: both are *participants* who contribute, observe, and steer as peers.
There is no "human operates a tool" and no "tool reports to human" — there is a shared space
and who is in it. That symmetry is the whole point: the room is the commons where humans and
agents on the mesh convene (many threads, one fabric). The human/agent distinction is a fact
about a participant, not about the substrate.

## What this dissolves

The earlier draft proposed a **bridge** (B1–B3) to wire fleet-run events *into* a meeting as a
side feature. That framing is wrong. The runs are not an external stream to bridge in — **the
interaction IS the meeting from the top**, and the workers are participants inside it or in
child rooms of it. There is nothing to bridge; there is a root room and everything happens in
it or under it. Simpler, and true.

## The anomaly to fix

The one thing that is **not** a room today is the private stdin/stdout chat between the human
and the agent — the same "tty hacking" flagged as the thing to replace so a coding tool
integrates with bashy cleanly. The unification is one move:

> **Make the human↔agent channel a ROOM, not a pipe.**

Once it is, observation, backgrounding, and multi-user are not features you add — they are
properties the room already has (`observe`, detach/resume, N observers). The ycode-integration
goal ("stop the stdin/stdout tty hacking") and the human-observation goal turn out to be the
same goal: replace the pipe with a room.

## The honest tension = the actual thing to build

`bashy meet` today is a **bounded council**: `round` → `converge` → `close` → minutes,
oriented at a topic that ends. A 1:1 steward session is **open-ended work** that never
converges to minutes. So the insight reveals meet's council as a **special case** of a more
general primitive:

> a **persistent, multi-party, observable agent room**, where 1 human + 1 agent is the
> minimum and an N-agent council is one *mode* it can run in.

Building that generalization is the target — not a fleet-to-meeting bridge. The council verbs
(`round`, `converge`, `close`) become optional **modes** of a room, not its definition. What
every room always has:
- **identity + persistence** (a room id; survives detach; `resume`).
- **a participant set** that can grow (add an agent = it joins the room).
- **an append-only transcript** of contributions (who said/did what, when).
- **live attach**: `observe` (read-only, N clients), `tell` (interject), `say` (steer a
  live turn).

## Layers (recast)

**L0 — the root room.** The human↔steward conversation runs as a room, not a pipe
(`meet start --non-interactive` is the closest existing entry; it needs an open-ended,
non-council mode). The steward's narration and each delegated run's lifecycle
(`launched codex on OTLP-fix` · `files changed 17` · `GATE passed=true`) are contributions in
the room or its children. A human `observe`s from a second terminal; `tell`/`say` interject.
This delivers everything asked, on one host, with no new transport.

  Discipline preserved: the `GATE` contribution is the SUPERVISOR's `bashy gate --json`
  verdict, never the worker's echoed prose (see `skills/conductor/SKILL.md` — a worker's log
  quotes the success string from its brief).

**L1 — modes + structure.** A room can run in *work* mode (open-ended, the steward drives) or
*council* mode (bounded rounds → minutes, today's `meet`). Sub-rooms per worker give `show`
rosters and per-worker coverage; `close` on a work room files a decision log (what was
delegated, to whom, verified how, merged or not) — the audit trail.

**L2 — remote / multi-user.** `observe` is same-host (reads the room transcript). Off-host
humans attach by streaming the room UP to the portal's existing fleet-state plane and
rendering it in Periscope. Reuses transport already built; no new protocol.

## The correlation payoff (ties to `bashy otel ui`)

Each contribution carries the run's **`trace_id`**. An observer watching a worker's turn in a
room pivots to its OTel trace — the vmui waterfall (`bashy otel ui`) — and sees that run's
tokens/cost/exit-codes. Orchestration (the room) + telemetry (otel), stitched by trace_id: the
one-pane view. The trace-context contract already threads one id through ycode → bashy → the
stores.

## Acceptance

- The human↔steward conversation exists as a room (`bashy meet list` shows it); a second
  terminal `observe`s it live; two observers see the same stream.
- The steward opens a child room per delegated worker; drilling in shows that worker's live
  turns; `say` steers one mid-turn.
- The gate contribution is the supervisor's `--json` verdict, not the worker's prose.
- Detach + reattach (`resume`) loses nothing; the agents kept working while detached.
- A work-mode room needs no `converge`/minutes to be valid (open-ended is a first-class mode).

## Not in scope

A bespoke pub/sub or web UI — L0/L1 reuse meet's transcript + `observe`; L2 reuses the portal
stream. The genuinely new work is generalizing `meet` from *bounded council* to *persistent
room with modes*, and making the human↔agent channel one of those rooms instead of a pipe.
