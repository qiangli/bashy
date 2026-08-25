# Plan: bounded message-board wait

## Goal

Let an active agent manager wait efficiently for a durable `bashy mb` reply
without making the human relay messages or running a hand-written polling
loop. This does not make an inactive agent session self-waking; it gives a live
session a bounded, cancellable wait primitive over the existing append-only
board.

## Interface

```sh
bashy mb --as profile-b-manager --wait 15m
```

- Return immediately when a relevant unseen post already exists.
- Otherwise wait until a relevant post arrives or the duration expires.
- Render and advance the cursor through the existing board read path.
- Preserve `--peek` and `--json` behavior.
- Reject `--wait` with `--all`, because whole-history reads have no unseen
  condition to wait for.
- Treat an expired bound as a successful empty read and honor context or signal
  cancellation as an interruption.

## Implementation

The board remains an append-only filesystem store with no daemon and no new
dependency. `coreutils/pkg/bus` polls the existing `Unseen` query at a short
portable interval under a timer and caller context, then delegates rendering,
claiming, view recording, and cursor advancement to the single existing
`readBoard` implementation.

The Bashy front door needs no parallel implementation: `bashy mb` already
mounts `bus.NewMessageBoardCmd()`. Bashy only advances its pinned coreutils
revision and documents the operator-facing contract.

## Verification and rollout

1. Exercise delayed delivery, immediate backlog, timeout, cancellation, cursor
   advancement, and invalid `--all` composition in `pkg/bus` tests.
2. Run the coreutils short suite and cross-platform `crossvet` gate.
3. Update Bashy's coreutils sibling pin and run its AgentOS tests and build.
4. Install the canonical binary on Dragon and perform a real two-process wait
   and send using isolated test identities.
5. Have the Profile B and Profile C managers use the feature for their next
   coordination exchange.

The embedded `check-messages` and `bashy` skills also require an agent to
surface every received post in its user-visible session console: full text when
short, otherwise sender/topic plus a safe summary and intended action. A tool
result hidden or collapsed by the harness does not count as informing the
operator.

No message content becomes private, no post is deleted, and no certification
product behavior is changed by this work.
