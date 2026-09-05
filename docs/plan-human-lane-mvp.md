# Human lane MVP — join a meet, chat a manager, post on mb

**Status: plan. 2026-09-05.** Scope is the FIRST implementation: let a human
participate in the coordination surfaces that already exist. It adds no new
message store, no new transport, and no model to the send/receive path.

## The one capability

A human (`qiangli`) can, from ONE identity:

1. **join any known meet** and be seen in its roster;
2. **chat a project manager** — a sprint owner — in the room or by private DM,
   and learn immediately whether it was delivered;
3. **post and read on `bashy mb`**, with the human lane readable.

## Repository boundary

This is dhnt-only work. The sprint tracks the two umbrella subprojects that own
the existing surfaces:

- `bashy/` owns the shipped front door, principal wiring, unified human inbox,
  and `bashy apps serve` integration;
- `coreutils/` owns `pkg/meet`, `pkg/bus`, and `pkg/weave`, including the room,
  delivery-receipt, and sprint-reachability primitives.

The Sprint 126 cards stay together in `bashy/docs/todo`, but their
`Implementation home` lines below are authoritative for code placement. No
external service, unrelated repository, new transport, or foreign issue is a
prerequisite.

## What is actually broken today (probed 2026-09-05, not assumed)

| # | observed | surface |
|---|---|---|
| 1 | `bashy inbox` refuses with *"a human meaning to speak as themselves here passes `--as qiangli`"*, while `bashy inbox human` keys off the OS user, and `meet dm --as` defaults to `BASHY_AGENT_ID`, which a human does not have | identity |
| 2 | `bashy meet join` does not exist. `invite` is organizer-only, `observe` is read-only, `tell` presumes a seat | meet |
| 3 | The board carries `[UNREACHABLE: 2]` on #86, #100, #101, #122 and `STALE` on #89, #101, #106, #115. `bashy agents` shows **1 live agent, 3 stale**. A message to those owners is written and never read | sprint / inbox |
| 4 | `inbox human` shows duplicated `bus/fleet.unresolved` broadcasts, three of them with an **empty sender** (`bus:3097`, `3098`, `3104`) | inbox |
| 5 | A human-authored message has no delivery state the sender can see | all |

## The design rule

Nobody here reasoned badly. A message was written, was durable, and nobody read
it — `[UNREACHABLE: 2]` on the board is exactly that, and a human walking into
these surfaces today hits the same wall from the other side. So the MVP adds two
things around the surfaces that already work: **say who can receive, before you
send** (story 4), and **say what happened to what you sent** (story 6).

Three constraints follow, and they are what keeps this MVP small:

- **No model anywhere in this path.** Reachability, cursor position and ack state
  are all decidable from state already on disk: a live watcher either exists or it
  does not, an `inbox-ack` either happened or it did not. Nothing here needs a
  judgement call, so nothing here gets one.
- **Audit only, never a repair loop.** Delivery state is reported to the sender
  and to no automation. Nothing retries, re-words, escalates or re-routes on the
  strength of it — an auto-retry would make a dead owner look reachable, which is
  the precise condition this sprint exists to expose.
- **Nothing new to run.** No new store, no new transport, no new web surface.
  Every story is one verb or one column over state the host already keeps. The
  existing `bashy apps serve` browser path is a required regression consumer of
  the same identity and delivery contract, not a separate implementation.

## Master execution plan

Execute priority-first, with no fleet fan-out required:

1. **S1 — identity (`bashy` + `coreutils`).** Establish one canonical human
   principal and prove agent cursors cannot consume its state.
2. **S2 — join (`coreutils`, then bashy exposure).** Extend the existing
   `meet join` contract from coreutils story `0892cb5be1d3`; do not create a
   second join path or let a durable seat claim live-process reachability.
3. **S3 — truthful send (`coreutils` + bashy wiring).** Reuse the existing bus
   delivery classifier for room and DM sends; return a reasoned verdict while
   retaining the durable record.
4. **S4/S5 — legibility (`coreutils` / `bashy`).** Add the pre-send owner
   inventory and clean the human inbox. They may proceed independently after
   S1 and S3 settle the shared identity and receipt shapes.
5. **S6 — audit convergence (`bashy` + `coreutils`).** Normalize
   sent -> delivered -> acked across mb, meet, DM, inbox, and the existing apps
   browser surface; prove there is no retry, re-word, escalation, or reroute.
6. **Final umbrella gate.** From one human identity, use both the CLI and
   `bashy apps serve` to join a known room, room-chat and DM a live manager,
   obtain a reasoned NOT DELIVERED result for an unreachable manager, post/read
   mb, and observe the same receipt advance to acked. Run focused package tests,
   then `go test -short ./...` in `coreutils` and `bashy`.

## Explicitly OUT of scope for the MVP

- Any model drafting, answering, routing or triaging on the human's behalf.
- Automatic retry, re-wording, or escalation of an undelivered message.
- A new web surface: `bashy meet serve` and `bashy apps serve` already exist and
  are the browser path. Sprint 126 must exercise them end to end; implementation
  there is limited to compatibility wiring needed to expose the shared contract.
- Fixing the STALE/UNREACHABLE owners themselves. This sprint makes their state
  legible to a human and lets that human step in; recovering an agent's delivery
  capability belongs to the sprint that owns the agent.

Each is deferred for the reason above, not for cost.
