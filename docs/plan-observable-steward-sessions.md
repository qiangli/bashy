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

## The room is a first-class construct — and transport-agnostic by design

`room` is a **key bashy construct**, a peer of the shell and the fleet — NOT a mode of
`meet`, and NOT a local/TUI feature. This is a hard design rule, because the tempting
mistake is to build it as "a TUI you attach to locally" and bolt remote on later. That is
backwards. Bake in the wrong assumption and every surface after the first fights it.

A room is defined by two things only, both surface-independent:

1. **State** — its identity, participant set, and append-only transcript. This lives
   somewhere addressable (a host, later a mesh location); it is not tied to any terminal.
2. **A protocol** — the events a participant sends (contribute / steer / join / leave) and
   receives (transcript deltas, presence, state). One protocol, spoken by every surface.

Everything a *human* uses to be in a room — a local TUI, a remote CLI, a **web UI**, a
**mobile app** — is a **participation surface**: a client that speaks the room protocol and
renders it. They are equal citizens, designed in from the start:

| surface | how it speaks the protocol | for |
|---|---|---|
| local TUI | in-process / same-host attach | the operator at the machine |
| remote CLI | the protocol over the mesh/tunnel | an operator on another box |
| **web UI** (Periscope) | the protocol over WebSocket through the portal | anyone with a browser, no install |
| **mobile app** | the same protocol over the same WebSocket | remote participation from a phone |

This is exactly the dhnt mobile stance: a phone is a **client into the mesh**, not a node —
and *participating in a room* is precisely that client role. A human on a phone joins the same
room a local TUI and three agents are already in, and contributes/observes/steers as a peer.

The transports already exist and must be REUSED, not reinvented: same-host attach for local;
the matrix tunnel + the portal's fleet-state stream for cross-machine and web; the same
WebSocket for mobile. The room protocol rides these; it does not invent a new one.

**Design consequence:** the very first cut must put the protocol/state at the core and make
even the local TUI *a client of it* — so the web and mobile surfaces are additional clients,
never a re-architecture. If the local TUI reaches into room internals directly, remote is
already lost.

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

## What `bashy meet` already provides (the head start — assessed 2026-07-15)

The room construct is NOT a from-scratch build. `coreutils/pkg/meet` already has the hard
part, and — crucially — its design is already transport-agnostic *at the data level*:

- **The two-channel model IS the room state model, done right.** `transcript.jsonl` is the
  RECORD (append-only, one sanitized `Event` per completed turn — the canonical state, what
  minutes and prompt-context are built from). `live.jsonl` is the VIEW (ephemeral,
  line-granular, derived, safe to lose — every line also lands whole in the transcript). Record
  vs view is exactly the split a surface-independent room needs.
- **Multi-observer already works.** `observe` tails those files; "any number of observers can
  attach to the same meeting at once" (`observe.go`). Room identity exists (`room.go` — the
  ROOM number you attach by). `say`/`steer` exist.
- **The state is clean JSONL, not a PTY.** The channels are structured events on disk, not a
  terminal capture — so a second transport is an adapter over the SAME events, not a rewrite.

So the real gaps are narrow and known:

1. **A network surface.** Today `observe` tails a *local file*; a web/mobile/remote client
   cannot. The fix is a thin adapter that streams the SAME `transcript`+`live` events over
   **WebSocket** — a new surface over existing state, the JSONL event schema already being the
   protocol. This is S2 below, and it is the load-bearing new work.
2. **Work mode.** meet is *council* mode (bounded planning meeting; `engine.go`'s turn guard
   says "a participant in a planning meeting… one concise turn"). The open-ended 1:1
   human↔steward session is a new *mode* of the same room, not a new construct.
3. **The 1:1 base case.** meet convenes N-agent councils; a two-participant open-ended room
   (a human + one agent) is the shape to add.

Net: the record/view state, room identity, multi-observer, and steering are DONE. The build is
(a) a WebSocket surface over the existing event channels, and (b) a work mode + the 1:1 shape.
That is a much smaller, better-grounded scope than "build a room."

## Build order (NOT a local-then-remote layering)

The sequencing is by *construct*, not by *reach* — the protocol is the foundation, and every
surface (including the local TUI) is a client of it from day one. This is deliberately not
"ship local, add remote later," which is the trap the transport-agnostic rule forbids.

**S1 — the room core: state + protocol.** Define the room construct — identity, participant
set, append-only transcript — and the event protocol (contribute / steer / join / leave /
transcript-delta / presence). This is the load-bearing piece; get it surface-independent and
everything else is a client. The human↔steward conversation becomes a room here (an
open-ended *work*-mode room, distinct from `meet`'s bounded council). Run-lifecycle events
(`launched codex` · `files changed 17` · `GATE passed=true`) and steward narration are
contributions; sub-rooms per worker nest under it.

  Discipline preserved: the `GATE` contribution is the SUPERVISOR's `bashy gate --json`
  verdict, never the worker's echoed prose (`skills/conductor/SKILL.md` — a worker's log
  quotes the success string from its brief).

**S2 — the first two clients, proving the protocol is surface-independent.** The **local
TUI** (`observe`/`tell`/`say` as clients of the protocol, not reaching into internals) AND a
**WebSocket endpoint** exposing the same protocol. Two surfaces from the start is what keeps
the core honest — if only the local TUI exists, the abstraction rots toward it.

**S3 — the remote/web/mobile surfaces, which are now just more clients.** A browser
(Periscope) and the mobile app speak the S2 WebSocket protocol over the portal + matrix
tunnel — cross-machine, no install, remote participation. Because S1/S2 were built
surface-independent, these are *additions*, not a re-architecture. `close` on a work room
files a decision log (what was delegated, to whom, verified how) — the audit trail, available
to every surface equally.

**Modes, orthogonal to surfaces:** a room runs in *work* mode (open-ended, a driver leads) or
*council* mode (bounded rounds → minutes, today's `meet`). Modes are room behavior; surfaces
are how you attach. The two never entangle.

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

## Reuse, don't reinvent

The room protocol rides transports that already exist — same-host attach, the matrix tunnel,
the portal's fleet-state stream, WebSocket — it does not invent a new one. The genuinely new
work is three things: (1) the **room construct** itself (state + surface-independent protocol)
as a first-class bashy concept; (2) generalizing `meet` from a *bounded council* into one
*mode* of a persistent room; and (3) making the human↔agent channel a room instead of a pipe.
The web (Periscope) and mobile surfaces are not out of scope — they are the point — but they
are *clients of the protocol*, built after and on top of it, never a second architecture.
