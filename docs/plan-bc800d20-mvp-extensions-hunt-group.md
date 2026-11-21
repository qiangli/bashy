# Plan: bc800d20 — Direct `NAME+N` Extensions, `NAME` Main Line Hunt Group, and Turn Delivery vs Ack

## Scope (Operator Scope Cut 2026-11-21)

- `NAME+N` is a direct, self-standing address: own UUID-keyed mailbox, own read cursor, own caller ID (`from NAME+N`), and `--as NAME+N` for read and send. A reused `+N` starts empty.
- `NAME` main line is a hunt group: messages addressed to `NAME` wait when no instance is live and are claimed atomically (exactly once) by a live instance (`bus.ClaimPost`/`ClaimHolder` + live-session proof).
- Turn delivery (`DeliveredAt`) is tracked separately from explicit acknowledgement (`AckedAt`), so withheld `'1 of N unread'` items stay unread and retrievable via `bashy inbox`.
- Preserves existing named seats, vendor default model behavior, observational peeks, and `--as` generation/authorization protections.
- Deferred out of scope: forwarding ended extension mail, team-lead derived extensions (`LEAD+runN`/`LEAD/ext`), `NAME+*` ring-all, company number (`193d49d8`).

## Changes

### Yoke (`github.com/qiangli/yoke`)
1. `pkg/admission/admission.go`, `pkg/bus/turn.go`: add optional `Item.MarkDelivered` so turn boundary commits mark shown items delivered without acknowledging withheld items.
2. `pkg/bus/mainline.go`, `pkg/bus/recipient.go`, `pkg/bus/send.go`, `pkg/bus/board.go`, `pkg/bus/messageboard.go`:
   - Parse `+N` extensions (`ParseExtensionLabel`).
   - Resolve `NAME+N` strictly to its live `fleet.Instance` UUID (`retiredError` if retired, never forwarded).
   - Resolve main-line `NAME` to a hunt-group `Post` (`To: NAME, Mode: ModeAny`), preserving existing non-line family ambiguity and retired-label refusals.
   - Enforce live-session proof (`ReaderHasLiveSession`) and atomic `ClaimPost` on non-peek reads (`mb`, `inbox --watch`, turn delivery commit), keeping `--peek`/`--history` observational.
   - Enforce `MailFrom` watermark and `ClaimHolder` in `Post.directed`/`FilterPostsForReader`/`unseen`.
3. `pkg/bus/actor.go`, `pkg/bus/board.go`: support `--as NAME+N` and self-attribution (`from NAME+N`, `FromParty` snapshot) with strict UUID generation matching via `SelfInstanceUUID()` and live session claims.
4. `pkg/chat/interact.go`: fix `joinNextInstance` updating `card.Nick` and `Launch.Nick`, and bind/retire a `fleet.Instance` per live session (preserving vendor default model when no `--model` is given).

### Bashy (`github.com/qiangli/bashy`)
1. `internal/agentos/mailbox.go`, `internal/agentos/inbox_turn.go`, `internal/agentos/inbox.go`:
   - Track `DeliveredAt` separately from `AckedAt` in `mailboxMark`/`mailboxItem`.
   - Separate turn-boundary snapshots (`forTurn = true`, skipping `DeliveredAt`) from `bashy inbox` CLI reads (`forTurn = false`, keeping delivered/withheld items unacked).
   - Support `NAME+N` and instance UUIDs in `resolveInboxReader` (strictly matching caller's own verified generation), `resolveInboxWatcherIdentity`, `recordInboxPeekClaim`, and hunt-group `ClaimPost` in `inbox --watch` / `inbox ack`.
2. `internal/agentos/messageboard.go`, `internal/agentos/agentos.go`:
   - Wire `bus.IsMainLineTool` in `wireMessageBoard`.
   - Update `wrapAuthoredClaimErrors` not to report an unclaimed seat as held live when `holderPID == 0` and to inspect instance cards.
3. Hermetic unit & concurrency tests in Yoke and Bashy covering all 5 acceptance cases plus attribution/generation refusal tests.
