# Retro: Sprint 562 manager handoff

The Sprint 562 manager seat moved from `claude-opus5.5-s562` to `codex-gpt6-sol`
on 2026-11-04 at the operator's request. Brief: `s562-manager-handoff.md`.
Hosts are named by role. This retro feeds Sprint 568 (D1 capability map, D4
handoff behavior) and the live-handoff sprint (`plan-live-handoff.md`).

## What worked

- A written brief and a joint first step (a Linux corpus run that the successor
  reviewed) gave the successor real context before it took the seat.
- `sprint handoff` recorded continuity and released the lease. Linked weave
  workers kept running through the switch.
- Each step was acknowledged explicitly by both managers.

## What hurt

Roughly 1.5 hours of overhead went into what should be a five-minute act.

1. **Routing.** `mb send <agent>` was refused as ambiguous when the family had
   several live instances. Mail addressed to an instance UUID did not appear in
   the recipient's name-based inbox. A session UUID was not routable. The board
   accepted messages with "no read evidence", so the sender could not tell what
   had arrived. The operator relayed messages between the sessions by hand.
2. **No handoff object.** The brief was free text assembled from about eight
   commands: worker roster, tree SHAs, held merges, logs, claims and pending
   requests. The successor still had to ask for statuses, SHAs and logs.
   Nothing listed the outgoing manager's obligations, such as a pending push
   acknowledgement request from another sprint.
3. **No offer/accept protocol.** The sequence was handoff (lease released), then
   take, then claims released and re-claimed. That left a window with no owner,
   and nothing tied "successor accepted" to the transfer itself.
4. **Partial transfer.** Host claims moved by hand (release, then claim). Story
   assignees and linked runs still showed the old manager.
5. **Tooling defects** (owned by Sprint 563 F9-F9d):
   - Watch, peek or send under a clone name seated the parent agent.
   - The lease went stale even though checkpoints reported "lease refreshed".
   - `sprint take` hung for minutes.
   - `handoff` refused until pending bus messages were read, with an unclear error.
6. **People were not first-class.** The operator could not address, acknowledge
   or watch the handoff directly.

## Follow-ups

- Local handoff mechanics (snapshot, offer/accept/decline, atomic transfer):
  live-handoff sprint, stories H1-H5.
- Identity, addressing and cross-service delivery: Sprint 568 (design).
- Seat, lease and inbox defects: Sprint 563 F9-F9d.
