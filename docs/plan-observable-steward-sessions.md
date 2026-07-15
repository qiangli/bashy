# Plan: observable steward sessions via `bashy meet`

*Design, 2026-07-15. Grounded in a real gap: a whole fleet-supervision session ran
through raw `nohup` + private monitors, so the human user could only see it through the
steward's chat prose. This makes it observable — another terminal, backgroundable,
multi-observer — by modelling the steward session as a meeting.*

## The insight

A **steward session** (one agent orchestrating a fleet of worker agents to a verified goal)
and a **meeting** (`bashy meet`) are the same shape: a chair, participants, a live
transcript, and observers. `bashy meet` already ships every primitive the observation goal
needs:

| want | `bashy meet` verb |
|---|---|
| watch live in another terminal | **`observe`** — attach, read-only |
| many people watch the same session | multiple `observe` clients on one room |
| leave the steward running in the background | **`start --non-interactive`** (detached) |
| a human interjects guidance | **`tell`** (append a human contribution) |
| a human steers a worker mid-run | **`say`** (steer the agent currently speaking) |
| the durable record afterwards | **`close`** → secretary minutes; **`contributions`** = full transcript |

So the steward session becomes: the steward is the **chair**; each worker agent (codex,
ycode, agy, opencode) is a **participant**; the orchestration itself is the **deliberation**;
and the human `observe`s.

## Why meet, not the sprint board

They answer different questions and compose:

- **`sprint`** is a durable *task* board — backlog, runs, verdicts, resumable. Answers
  *"what is the state of the work?"*
- **`meet`** is a live *transcript* you attach to. Answers *"what is happening right now, and
  let me interject."*

The user's goal is **observation** — watch it happen, in another terminal, backgroundable,
multi-user — which is meet's shape, not the kanban's. Best of both: **a steward session IS a
meet (the live window) that MAY drive a sprint (the durable task state).** Start with meet.

## The gap (what to build)

meet's participants are agents deliberating a *topic*; a steward session is the steward
*orchestrating workers*. The bridge is the work:

**B1 — fleet-run lifecycle → meet contributions.** Today a delegated run is a raw `nohup`
with a private monitor. Each run must instead POST its lifecycle into the meeting transcript
as contributions:
  - `launched: codex on OTLP-fix (branch otel-exec-not-link)`
  - `files changed: 17` · `context total=54568` (progress ticks)
  - `GATE (supervisor-run): passed=true` — the verdict the steward produced, NOT the worker's
    prose (the `bashy gate --json` discipline).
This is exactly the private-monitor stream I hand-rolled this session, redirected from my
chat into a shared, attachable transcript. The natural home is the launcher: whether a run
goes through `bashy weave` (preferred) or a thin `meet`-aware launch shim, its events fan
into the room.

**B2 — the steward narrates into the meeting.** The chair's decisions ("verified codex's
gate myself → passed; diagnosed the OTLP path doubling; delegated the fix back to codex")
become contributions via a steward→`meet tell` hook, so an observer sees the *reasoning*, not
just raw run events.

**B3 — worker = participant identity.** Register each worker (`codex:gpt-5.6-sol`,
`ycode:glm-5.2`, `agy`) as a participant so `show` gives the roster + per-worker coverage and
`observe` labels contributions by who did them.

## Layers

**L0 — local, mostly wiring (the 80%).** `bashy meet start --non-interactive` opens a
"session room"; the steward runs detached; B1+B2 fan run-events and steward-narration into
the transcript. A human runs `bashy meet observe <room>` in another terminal — live,
read-only, and **as many observers as want to attach**. `tell`/`say` give interjection. This
alone delivers everything the user asked for on one host.

**L1 — structure.** Workers as formal participants (B3); gate verdicts as typed contributions
(pass/fail + the failing checks); `close` emits session minutes (what was delegated, to whom,
verified how, merged or not) — a durable audit trail of the steward's decisions.

**L2 — remote / multi-user (other machines).** `observe` is same-host (it reads the room's
transcript). For users on OTHER machines, stream the room UP to the portal (the existing
`/sprints`-style push plane) and render it in Periscope — the web observation surface. Reuses
the cloudbox streaming already built for fleet state; no new transport.

## The correlation payoff (ties to `otel ui`)

Each contribution carries the run's **`trace_id`**. An observer watching a worker's turn in
`meet observe` can pivot to its OTel trace — the vmui waterfall (`bashy otel ui`) — and see
the tokens/cost/exit-codes of that exact run. Orchestration (meet) + telemetry (otel),
stitched by trace_id: the one-pane view. The trace-context contract already threads one
trace_id through ycode → bashy → the stores, so the id is available to attach.

## Acceptance

- The steward launches a session as a detached meet; `bashy meet list` shows the room.
- From a second terminal, `bashy meet observe <room>` streams live run events + steward
  narration; two `observe` clients see the same stream.
- `bashy meet tell <room> "pause codex"` reaches the steward; `bashy meet say` steers a live
  worker.
- The gate verdict in the transcript is the SUPERVISOR's `bashy gate --json` result, never the
  worker's echoed prose (the discipline from `skills/conductor/SKILL.md`).
- `bashy meet close <room>` files minutes naming each delegation + its verified outcome.

## Not in scope

Building a bespoke pub/sub or web UI — L0/L1 reuse meet's existing observe/transcript, L2
reuses the portal's existing stream. The only genuinely new code is the **bridge** (B1–B3):
run lifecycle → meet contributions, and steward narration → `meet tell`.
