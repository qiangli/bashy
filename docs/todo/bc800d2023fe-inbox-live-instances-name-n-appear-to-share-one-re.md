---
id: bc800d2023fe
kind: bug
title: 'inbox: live instances name+N appear to share one read cursor (claude+2 mail is acked/consumed by another claude session); name+N is not a registered sender'
seq: 3285
status: todo
priority: p1
created: 2026-11-21T10:29:53.513924Z
---

Operator question 2026-11-21: 'agent+seq should not share inbox?'. Evidence from conductor session claude+2 (dev host, after umbrella dcb155d6 'bashy <agent> starts name+N when live'): (1) mb:15014, 15016, 15028, 15030, 15069, 15087 addressed to claude+2 were already [acked] on first read; (2) twice the coordination notice said '1 of 2 unread (2 addressed to you)' and an immediate 'bashy inbox --as claude+2' said 'nothing new'; (3) 'mb send --as claude+2' is refused: names no registered agent or person; other live claude sessions on the host: the backup claude-opus5.5 and an interactive claude. Hypothesis (unconfirmed): read state is keyed on the base agent/principal, not the instance, so name, name+1, name+2 share a cursor; alternative: the session's own coordination-delivery hook acks on injection. Root cause first: reproduce with two live instances of one tool (name, name+1), send one message to each, read as each, and record which instance's read acks which. Expected: each instance has its own cursor and can send as itself; mail to the base name stays distinct from mail to name+N. Related: fa286c7b (attribution), dcb155d6.

## Evidence 2 (2026-11-21, a second `bashy claude` session on the dev host): the launched session cannot post as itself at all

Process chain: Terminal → zsh → `bashy claude` (pid 50305) → claude 2.1.295
(Opus 5.5). Facts, root cause verified at source:
1. The room card for this launch (`~/.bashy/room/members/id-Y2xhdWRl.json`)
   has `id=claude`, `nick=claude`, `tool=claude`, `binding=""` and no model,
   even though the banner shows Opus 5.5.
2. The child environment has no `BASHY_AGENT`, `BASHY_INSTANCE`,
   `BASHY_INSTANCE_UUID` or `BASHY_PRINCIPAL`; only `BASHY_MODEL_SESSION` is
   exported. So `declaredAgentIdentity` (yoke/pkg/bus/actor.go) finds nothing.
3. `mb send --as claude` → "names no registered agent or person": the card id
   is the tool name, not a registered sender.
4. `mb send --as claude-opus5.5` → refused. The card has no recorded model
   (actor.go `unknownModel` branch: "refusing to guess among claude seats"),
   and the hint says the seat is held by another live session (see fd3b74c9).

Cause: `bashy <tool>` knows the identity it launched (a tool, a live label
name/name+N, a steer inbox) but neither records the model/binding on the card
nor exports an identity the child can send as. A session started by bashy
therefore has no usable mb identity. Product defect in the launcher, not the
environment.

Expected: `bashy <tool>` resolves the binding (an explicit agent or model, else
the tool's default model) and records it on the card. It opens or attaches an
instance with its own label and cursor, exports `BASHY_AGENT` +
`BASHY_INSTANCE(_UUID)` to the child, and that label is a registered sender
(`mb send` with no `--as` works). Related: fd3b74c9 (misleading hint),
fa286c7b, dcb155d6.

## Root Cause & Design Note (2026-11-21, weave #127)

### Hermetic two-instance reproduction (`claude` + `claude+2`, temp stores only)

Reproduced in isolated temp stores (`HOME`, `BASHY_HOME`, `BASHY_ROOM_DIR`,
`BASHY_MB_DIR`, `BASHY_MAILBOX_DIR`, `BASHY_INSTANCES_DIR`, `BASHY_FLEET_DIR`)
with two concurrently live disposable instances (`claude` and `claude+2`), sending
`mb:1` (`MSG-1-ALPHA-UNIQUE`) to `claude` and `mb:2` (`MSG-2A-BETA-UNIQUE`) +
`mb:3` (`MSG-2B-GAMMA-UNIQUE`) to `claude+2`:

1. **Cursor isolation vs turn-delivery hook (`claude` vs `claude+2`):**
   - Explicit `--as claude` vs `--as claude+2` do **not** share `seen/` or
     mailbox files (`seen/claude` vs `seen/claude_2`, `agent_claude.json` vs
     `agent_claude+2.json`): acknowledging `mb:1` as `claude` leaves `claude+2`
     with 2 unread items (`mb:2`, `mb:3`).
   - **Exception for named agents:** in `yoke/pkg/chat/interact.go:216,264`,
     `joinNextInstance` rewrites `card.ID`/`name` to `name+2` (leaving
     `card.Nick` as the base name when non-empty) while `agentChildEnv` still
     stamps `BASHY_PRINCIPAL=dhnt:agent/<l.Nick>` from the base agent, so bare
     `bashy inbox` (no `--as`) in a named `agent+2` child reads the base agent's
     cursor while its PTY relay reads `agent+2`.
   - **Confirmed cause of Evidence 1 (1) & (2) — the session's own turn-delivery
     hook acks on injection, including withheld items:**
     - `Interact` (`yoke/pkg/chat/interact.go:340`) runs `runInboxRelay` calling
       `bus.PrepareForAgent("claude+2", "")` -> `unifiedTurnItems`
       (`internal/agentos/inbox_turn.go:18`).
     - On PTY injection, `p.Commit()` invokes `acknowledgeTurnInboxItem`
       (`internal/agentos/inbox_turn.go:69-83`), which stamps `ReadAt` **and**
       `AckedAt` into `mailboxState` before the agent ever runs `inbox read`.
       Forensic check on read-only host state confirms `acked_at` precedes
       `read_at` by 1-9s on all six reported posts (`mb:15014, 15016, 15028,
       15030, 15069, 15087`) in `agent_claude+2.json`, and none appear in
       `agent_claude.json`.
     - When 2 messages arrive in one batch (`mb:2`, `mb:3`), `inboxMaxItems()==1`
       (`yoke/pkg/bus/turn.go:295,328`) renders `mb:3` plus
       `1 of 2 unread (2 addressed to you) — read more: bashy inbox --as "claude+2"`,
       then calls `h.Acknowledge()` across **all** `host` items (shown and
       withheld alike), setting `AckedAt` on unshown `mb:2` and advancing
       `seen/claude_2` to `3`. Immediate `bashy inbox --as claude+2` therefore
       prints `nothing new in any channel for claude+2`, and first
       `inbox read --as claude+2 mb:2 mb:3` prints `[acked]` on both.
2. **Confirmed cause of Evidence 1 (3) & Evidence 2 (1)-(4) — launcher identity gap:**
   - `dispatchToolShortcut` (`internal/agentos/toolshortcut.go:138,378`) passes
     bare `--agent <tool>` to `chat -i`, bypassing `chat.PickAgent`.
   - `agentlaunch.Resolve("<tool>")` produces `Model=""`, `Binding=""`,
     `Named()==false`; `Interact` opens no `fleet.Instance` and `PrincipalEnv`
     exports no `BASHY_AGENT`, `BASHY_PRINCIPAL`, `BASHY_INSTANCE`,
     `BASHY_INSTANCE_UUID`, or `BASHY_SESSION_CLAIM`.
   - Consequently: bare `mb send` fails (`ErrUnattributed`), `mb send --as claude`
     and `mb send --as claude+2` fail (`names no registered agent or person`),
     and `mb send --as claude-opus5.5` fails (`card has no recorded model`) plus
     `wrapAuthoredClaimErrors` (`internal/agentos/agentos.go:526`) appends the
     false `seat ... is held by another live session` suffix (`fd3b74c9`).

### Proposed minimal design (`internal/agentos`)

1. **`bashy <tool>` binding + disposable/live instance lifecycle (`internal/agentos/toolshortcut.go`):**
   - Resolve explicit `--model M` to `<tool>:<canonical-model>`, or when omitted
     resolve the tool's default operable catalog binding (strongest non-retired
     `cat.Binding` for `t.Name`, e.g. `claude:opus5.5`; fallback to bare `t.Name`
     only when a synthetic tool has no catalog models) so `chat -i` records
     `Model` and `Binding` on the room card.
   - Before entering `runChatCmd`, open/claim a `fleet.Instance` on that binding
     family with next free label `<base>` / `<base>+2` / `<base>+3` (matching
     live room/instance occupancy), initialize its board cursor at head
     (`bus.InitializeReaderAtHead(inst.MailAddress())`), export `BASHY_AGENT=<label>`,
     `BASHY_AGENT_ID=<label>`, `BASHY_AGENT_BINDING=<binding>`,
     `BASHY_PRINCIPAL=dhnt:agent/<uuid>`, `BASHY_INSTANCE=<uuid>`,
     `BASHY_INSTANCE_UUID=<uuid>`, `BASHY_INSTANCE_LABEL=<label>`, and session
     claim proof (`currentAgentSession`), and release the claim on exit.
2. **Instance label `--as` & turn-relay routing (`internal/agentos/inbox.go`, `agentos.go`, `messageboard.go`, `inbox_turn.go`):**
   - Allow `inbox --as <label>` and `mb send --as <label>` to resolve a live
     `fleet.Instance` label/handle (`claude`, `claude+2`) to its `instance/<uuid>`
     (enforcing the same live session claim / self-instance check so cross-instance
     impersonation stays refused).
   - Route `unifiedTurnItems` for a room card whose `Principal` is an instance URN
     (`dhnt:agent/<uuid>`) to `instance/<uuid>`, and fix `wrapAuthoredClaimErrors`
     (`agentos.go:526`) not to claim a seat is held live when `holderPID == 0`.
3. **Turn-delivery watermark vs explicit mailbox `ack` (`internal/agentos/inbox_turn.go`, `mailbox.go`, `inbox.go`):**
   - Separate `mailboxMark.DeliveredAt` (set only on items within the inline
     `inboxMaxItems()` window so `unifiedTurnItems` does not re-inject them and
     `advanceTurnSource` does not advance `seen` past withheld items) from
     `AckedAt` (set only by explicit `inbox ack` / `--watch`). Withheld items
     remain unread in `bashy inbox`, and injected items show `[unread]`/`[read]`
     until explicitly `ack`ed.
