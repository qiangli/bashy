# Plan: bc800d20 — Per-Instance Inbox Cursors, Turn-Delivery State, and `bashy <tool>` Identity

## Context & Verified Root Cause

Story `bc800d2023fe` (Sprint 652, weave #127) investigates two operator reports on dev-host (`claude+2` and `bashy claude`):

1. **Cursor isolation vs turn-delivery acknowledgement (`Evidence 1`):**
   - Two disposable instances `claude` and `claude+2` keep separate `seen/` and `mailbox` files when addressed explicitly, except that for named agents `chat.Interact` (`joinNextInstance`) leaves `l.Nick` at the base name so child `BASHY_PRINCIPAL` points to the base agent while the PTY relay polls `name+2`.
   - In `internal/agentos/inbox_turn.go`, `acknowledgeTurnInboxItem` stamps `AckedAt` into `mailboxState` upon PTY turn-preamble injection (confirmed in host `agent_claude+2.json`: `acked_at` precedes `read_at` by 1-9s on `mb:15014, 15016, 15028, 15030, 15069, 15087`), causing first `inbox read` to report `[acked]`.
   - When `> inboxMaxItems()` (default `1`) unread records arrive together, `yoke/pkg/bus/turn.go:328` invokes `Acknowledge` on **all** `host` items (both inline `show` and `withheld`), marking withheld messages `AckedAt` and advancing `seen` past them so immediate `bashy inbox --as claude+2` prints `nothing new`.

2. **Launcher identity gap (`Evidence 2`):**
   - `internal/agentos/toolshortcut.go` passes bare `--agent <tool>` to `chat -i`, leaving `Model=""`, `Binding=""`, opening no `fleet.Instance`, and exporting no `BASHY_AGENT`, `BASHY_PRINCIPAL`, `BASHY_INSTANCE(_UUID)`, or session claim to the child.
   - Bare `mb send`, `mb send --as claude`, `mb send --as claude+2`, and `mb send --as claude-opus5.5` all fail, and `wrapAuthoredClaimErrors` (`internal/agentos/agentos.go:526`) appends a false `seat ... is held by another live session` suffix when `holderPID == 0`.

## Minimal Changes (`internal/agentos`)

1. **`internal/agentos/toolshortcut.go`**:
   - Resolve `--model M` or default catalog binding for `<tool>` (strongest operable non-retired `tool:model` in `fleet.Catalog`, falling back to bare `<tool>` only when no catalog model exists for synthetic test tools).
   - Open/attach a live `fleet.Instance` with label `<base>` or `<base>+N` (`N >= 2`), initialize `bus.InitializeReaderAtHead(inst.MailAddress())`, export `BASHY_AGENT`, `BASHY_AGENT_ID`, `BASHY_AGENT_BINDING`, `BASHY_PRINCIPAL=dhnt:agent/<uuid>`, `BASHY_INSTANCE`, `BASHY_INSTANCE_UUID`, `BASHY_INSTANCE_LABEL`, and session-claim token, and release the claim on exit.
2. **`internal/agentos/inbox.go`, `internal/agentos/agentos.go`, `internal/agentos/messageboard.go`**:
   - Permit `--as <instance-label>` in `resolveInboxReader` and `mb`/`notify` authored-actor resolution by mapping active `fleet.Instance` labels/handles (`claude`, `claude+2`) to `instance/<uuid>` while preserving live session-claim enforcement.
   - Support `tool:model` bindings and launcher session tokens in `currentAgentSession`.
   - Fix `wrapAuthoredClaimErrors` not to report an unclaimed seat as held by another live session (`fd3b74c9`).
3. **`internal/agentos/inbox_turn.go`, `internal/agentos/mailbox.go`**:
   - Track `DeliveredAt` separately from `AckedAt` in `mailboxMark`, and apply `DeliveredAt` + source-cursor advancement only to items inside the inline turn-admission window (`inboxMaxItems()` + byte budget), leaving withheld items unread in `bashy inbox` and keeping `AckedAt` strictly for explicit `inbox ack` / `--watch`.
4. **Regression tests**:
   - Hermetic test with two concurrently live disposable instances (`name` and `name+2`), uniquely marked messages to each, independent observation/acknowledgement, withheld-item `1 of 2 unread` -> `bashy inbox` retrieval, and `mb send` as self (both bare and `--as <label>`).
