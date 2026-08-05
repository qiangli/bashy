# Running the steward: `bashy steward start` / `stop`

Design of record for the two verbs that turn the steward seat from a *record*
into a *post*, and for the message plane that makes a background steward reachable.

Companion docs: `docs/one-agent-control.md` (the control surface every
agent-driving verb uses), `docs/chat-interactive-launcher.md` (the governed
front door for launching a third-party CLI), `docs/agent-bands-and-nicknames.md`
(bands), `docs/absence-of-evidence.md` (the failure shape this design is built
against).

## The gap this closes

Everything under `bashy steward` before this read or wrote the **record**: who
holds the host, what they claimed, what nobody checked. None of it put an agent
*on* the host. So a machine could have a perfectly maintained journal and nobody
attending it, and every instruction that said "be the steward" was addressed to
a human who happened to be reading.

`start` selects an agent, takes the seat, opens the room, hands the agent its
predecessor's note, and leaves it running under a supervisor. `stop` asks for a
note, **checks that one arrived**, closes the room, releases the seat.

## Backgrounded, never headless

`handoff.Record.Role`'s doc comment states the constraint: a steward is
HOST-WIDE + INTERACTIVE, always, because a headless `--print` run is deaf to the
human and a deaf steward is the failure the seat exists to prevent.

That is honoured. The agent runs through `chat.Session` — the tool's
*interactive* launch under a pty with a control socket — detached into its own
process group. It is **backgrounded, not headless**, and stays addressable four
ways: the seat room (`meet`), the seat's bus inbox (`steward ping`),
`bashy chat attach <agent>`, and `bashy coach attach`.

## Selection: cost and quota, not alphabetical order

`bashy agents list --min-band 4` answers "who is strong enough". It does not
answer "which of them should I spend", and for a steward that second question is
the whole decision: a steward is a *process that stays up*, so it consumes its
model's quota for as long as the host is attended.

Ranking, on band-eligible candidates (`internal/agentos/steward_select.go`):

1. **billing class** — free (own hardware) → flat (a seat already bought, hard
   quota) → flat-then-metered (a seat that starts charging when spent) →
   metered. The fleet's standing "prefer subscriptions" rule, applied instead of
   remembered.
2. **measured headroom** — read from `llmbudget`, the *same* meter the budget
   gate enforces. An agent the gate would **Block** is excluded outright, with
   the reason printed: a seat that stalls on its first turn is an unattended
   host wearing a steward's name.
3. band, then reliability, then marginal cost, then name — so a rerun on an
   unchanged host picks the same agent.

**Unknown headroom is not full headroom.** A model with no recorded ceiling
sorts *below* a measured one with room to spare and *above* one measurably close
to its limit. Treating "no limit recorded" as "no limit" is the same class of
bug as an absent test result reported as a pass.

Every exclusion is printed. `--agent` wins outright and is only checked, never
overridden.

### The L3 floor

Below L3 you get a loud warning naming the *measured* failure rather than the
abstraction: a sub-L3 agent in an orchestrating seat does not fail cleanly, it
**loops** — repeats the same tool calls, never converges, and reports success
anyway (`docs/band-ladder.md`: gemini3.1 at 9.4× repeat ratio). A host stewarded
by one is worse than an unstewarded host, because it *looks* attended.

It is a warning, not a refusal: naming an agent is the operator's call.

## The seat: no shortcut was added

`start` picks claim-vs-takeover from the seat state so nobody has to know which
situation they are in — and then routes through the **ordinary authorized path**
(`steward authorize` + `claim`/`takeover`), including both typed confirmations at
a real terminal.

There is deliberately no unattended path and no `--yes`. A `start` verb that
could take host authority unattended would hand every cron job and runaway agent
loop on the machine precisely the capability those confirmations exist to
withhold. What `start` adds is that it prints the exact two commands to run.

Two cases skip acquisition legitimately:

- the seat is **already held live by this episode** — a restart of the agent is
  not a change of authority, so re-acquiring would burn an epoch for nothing;
- `$BASHY_STEWARD_EPOCH` is exported **and agrees with the journal** — the
  operator already claimed. Verified against the record, never believed from the
  environment.

`--no-seat` runs the agent unaccountable, and says so in both the console output
and the agent's own brief: its journal writes will be fenced and `steward ping`
will still report no steward.

## Already running

- **same agent** → a brief status line, nothing changes. The idempotent case: a
  supervisor tick, a second terminal, a script run twice.
- **different agent** → refused. Two stewards on one host is the ownership
  collapse the seat exists to prevent. The error names the incumbent and points
  at `steward ping`.
- **`--force`** → the incumbent is stopped *through the full wrap-up* (it is
  asked for a note first), then the new agent takes the post.

A session record whose supervisor pid is gone is **stale, not running**: it is
cleared with a message rather than blocking a start or letting a stop claim it
killed something already dead.

## The handoff note — three situations, three instructions

`start` surveys the store for a live `Role: "steward"` handoff and gives the
agent a *different* instruction for each outcome, because "there is no note" and
"the note is a day old" call for opposite first moves and an agent given one
instruction for both takes the reassuring reading.

| state | the brief says |
|---|---|
| **fresh** (< `--stale-after`, default 24h) | resume it — `bashy resume` then `bashy resume --claim` — and *verify before trusting it*: the note says what a predecessor believed |
| **stale** | treat it as a **lead, not a briefing**; run the investigation block; then record what is actually true, superseding it |
| **missing** | *not* permission to assume the host was idle. Reconstruct what was in flight, then write the note that should have existed — and say plainly what could not be established |

Staleness is 24h, deliberately shorter than `handoff.StaleAfter` (72h). That
constant governs a parked *task*, which is still true a week later — the diff has
not changed. A steward note describes a whole host, and a day of fleet activity
can invalidate all of it while leaving the note looking current.

The investigation block is the same list in both the stale and missing cases:
`steward reconcile` → `steward log --degraded` → `board` → `weave status` →
`chat sessions` → `resume --all` → `kb search` → per-repo `git log`/`status`.
The question ("what is actually in flight here?") is identical; only the reason
for asking differs.

## The message plane

A background steward is only useful if things can reach it. Three layers, and
the split between them is the design.

### 1. The bus sidecar — mechanical, free

`pkg/bus` already has a sidecar: it holds standing subscriptions, matches
topics, applies interrupt governance and the rate limit, and steers a live
session over its control socket. What it never had was **a process to live in** —
a host with no long-running bashy has no sidecar, so
`bus subscribe --interrupt-from steward` described a delivery tier that nothing
performed.

The supervisor hosts it (`--sidecar`, default on). It is the one process a
stewarded host keeps up for as long as it is attended, and it is already the
thing whose agent the sidecar would be interrupting. It costs no tokens.

### 2. The nudge — a pointer, never the messages

The supervisor polls the seat inbox and the message board and tells the agent
**counts and where to look**, at a turn boundary.

Not the bodies, and that is the one decision here worth defending. Both stores
**mark on read**. If the supervisor drained them and pasted the contents into
the prompt, the inbox would show every message read — by a process that is not
the steward — while the steward's own record showed it never looked. The channel
would report a healthy read side and nobody could tell that nothing had actually
been considered. So the agent runs `bashy steward inbox` / `bashy mb` itself and
the read is attributed to it.

The two channels are always reported **separately**. The seat inbox holds messages
addressed to the *role* and includes what predecessors were sent and never
answered; the board is the host's public channel. "You have 4 messages" loses
the only distinction that decides whether a predecessor's unanswered messages are
being read.

A nudge that could not be delivered does **not** advance the cursor — otherwise
a busy agent that missed one window never hears about those messages at all.

### 3. The mediator — a function, not a seat

The expensive part is **triage**: turning "6 new items" into "the api conductor
is blocked on a merge token; the rest is CI noise". That is a summarisation task
well inside an L2's competence, and buying it cheaply is worth doing.

It is a bounded **one-shot invocation** at a low band (`--mediator-band`,
default 2), fired only when new messages arrive, read-only, not a standing agent.

Why not a second seat:

- A seat costs a lease, an epoch ladder, a room, a journal, a heartbeat, a
  takeover path and a handover contract. The steward needs all of that because
  it is **accountable**. A message queue is not accountable; it is a queue.
- Two accountable seats on one host reopens the ownership question the steward
  skill spends pages closing. If a mediator decides what reaches the steward, it
  holds an authority the steward cannot audit — the steward would then judge the
  host on a filtered view it did not choose, and an unseen message reads as no
  message.
- The *watching* is already free. Nothing about polling needs a model.

**The contract: it summarises and ranks; it never filters.** The digest must
account for every new message, one line each, and the count is checked. Wrong
count, unparseable output, agent unavailable, budget blocked, timeout → the
digest is **discarded** and the plain mechanical pointer is sent instead, with
the degradation announced to the steward. A cheap agent is allowed to be
unhelpful; it is not allowed to make a message disappear.

Repetition cannot fake coverage either — the guard counts *distinct* message
indices, so six copies of "1." cover one message.

A mediator at or above the steward's own band is **refused**: it would cost what
it saves. The refusal is printed rather than silently doing the expensive thing.

## Stopping

`stop` runs the ending in the order that makes it useful:

1. **ask** the agent to record a note — as a *command*, explicitly, because
   prose in the transcript dies with the session and only a `bashy handoff`
   record survives it;
2. **verify** one actually landed. Asking is not being answered and an agent's
   own "done" is a claim, so `stop` looks in the store. If nothing is there it
   writes a **mechanical** record marked as such — a pointer at the journal
   saying a tenure ended untidily, never a briefing pretending to be one. The
   outcome file records `note_by: agent | fallback`, and the console says which;
3. **close** the room *before* the seat is released, so the host never
   advertises a channel to a seat nobody holds;
4. **release** the seat, fenced with the epoch it was held under.

`--force` skips step 1. The mechanical note is still written: a steward that was
killed is exactly the case a successor must not mistake for an idle host.

The two halves run in different processes — `stop` asks, the supervisor
performs — so the supervisor writes a `stop-outcome.json` the stop reads. The
previous outcome is **deleted first**: reporting a wrap-up that never ran using
the last stop's result would be the most convincing possible way to report a
success that did not happen. If the supervisor died without recording one,
`stop` performs the wrap-up itself and says so.

A **fenced heartbeat ends the tenure**: if somebody took the seat over, the
supervisor reports it and stands down rather than running as a steward the host
no longer recognises.

## Files

| path | what |
|---|---|
| `internal/agentos/steward.go` | the `start` / `stop` commands, the seat acquisition routing, the bootstrap brief and the wrap-up instruction |
| `internal/agentos/steward_select.go` | cost/quota-aware agent selection and the band floor |
| `internal/agentos/steward_supervise.go` | the supervisor: heartbeat, sidecar, message watch, wrap-up, fallback note |
| `internal/agentos/steward_mediator.go` | the cheap triage function and its coverage guard |
| `internal/agentos/steward_session.go` | the live-process record, the stop outcome, the handoff survey |
| `internal/agentos/steward_proc_{unix,other}.go` | detach / alive-probe / signal, per platform |

Session state lives beside the journal (`<steward dir>/session.json`,
`session.log`, `stop-outcome.json`) — never *in* it. A pid is the most
disposable fact in the system and the journal is designed to outlive the
hardware.

Two small coreutils additions carry it: `steward.{SeatContact,EnsureRoom,
AssumeRoom,ReleaseRoom,HolderName,Assignment}` (the seat-room lifecycle the host
owns but must not reimplement) and `chat.SessionOptions.AllowUnsafe` (the
session equivalent of `chat --yolo`, for a session nobody sits at).

## What the live run found (2026-08-05, dragon)

Eight `start → message → stop` cycles against real agents (codex-gpt-5.5,
ycode-gpt5.6-terra). **Every one of the following was a defect the automated
tests did not and could not catch**, which is the argument for running the thing
rather than testing around it.

1. **The wrap-up could never work.** The agent session ran on the same context
   that carries the stop signal, so SIGTERM terminated the agent's process tree
   *before* the wrap-up said "asking for a handoff note". Every stop fell
   through to the mechanical fallback while reporting that it had tried — a
   success path reached by the absence of the thing it was supposed to produce.
   Fixed by splitting `stopCtx` from `sessCtx`; the wrap-up now also states
   plainly when the agent was already gone.
2. **A message nudge could stall the heartbeat.** `WaitIdle` ran in the same select
   as the heartbeat ticker, so a busy agent blocked the pulse for up to
   `--nudge-wait` (10 min) and the seat would lapse while its holder was
   healthy — inverting the supervisor's entire purpose. Fixed: the message watch is
   its own goroutine, and liveness is checked on a separate 5s ticker.
3. **The mediator was off on every host.** It reused the seat's
   *strongest-first* ranking, so `--mediator-band 2` resolved to an L4, failed
   the "must be below the steward's band" check, and disabled triage. Fixed with
   a dedicated cheapest-first ordering; live runs now select `ycode-glm-5.2`
   (L2) under a `codex-gpt-5.5` (L4) steward.
4. **A room could be opened and never closed.** `EnsureRoom` reused a
   predecessor's saved contact, but meet lets only the *organizer* change a
   roster — so `stop` reported `room ... could not be closed` and the host went
   on advertising a live channel to a seat nobody held. Fixed by recording
   `role.Contact.Holder` and opening a fresh room when it differs.
5. **A silent watcher.** A delivered nudge logged nothing, so "delivered" and
   "never fired" were indistinguishable. Three rounds of live guessing went into
   that. The watch now announces when it arms (and what it primed against),
   when a message is seen, when it is delivered, and when push is not reaching the
   tool at all.

Also confirmed, and worth knowing:

- **A full disk killed an agent mid-session and produced no useful error.** The
  host was at 446 MB free; the agent exited ~4s after launch and the Go build
  failed inside the linker. This is exactly the case the steward skill's
  resource-health rule exists for — *do not trust evidence gathered under
  resource exhaustion*. 14 GB of derived Go build cache was reclaimed and the
  same run then worked.
- **`--yolo` is required for ycode's steerable launch.** Its template carries
  `--danger-skip-permissions` and the launch guard correctly refuses it
  otherwise. This is the flag doing its job, not a bug.
- **Push does not reach a continuously-repainting TUI.** codex never goes quiet
  for the 25s silence window, so no nudge is ever delivered to it. The message is
  not lost — it queues, and the agent reads it on its next `bashy steward
  inbox` — but the supervisor now says so once rather than retrying in silence.
  Only a tool that *reports* `turn.end` gets reliable push
  (`docs/first-party-harness.md`).

## Verified vs not

**Verified live:** install; selection with its cost/quota explanation; the L3
band warning; claim-vs-takeover routing; room open and close; the handoff survey
in its fresh and stale branches; detached spawn and supervisor lifecycle; the
launch guard and `--yolo`; mediator selection; **messages detected and delivered to
a live agent** (`messages — 1 seat` → `delivered a message notice`); `stop` and
`stop --force`; the wrap-up's honesty about an absent agent; the mechanical
fallback note; the stop-outcome file; stale-record clearing.

**Not verified:** the **message-board** channel has never been observed
non-zero. The cause is below this feature: on the test host the bus timeline had
been reset, so new messages reuse sequence numbers that already exist in the
persisted buffer (13 lines carrying 3 distinct seqs, 10 of them `3300`), and the
buffer's seq-dedup then drops them. `bashy steward inbox` shows the message
because it resolves live; the persisted view does not. That is a `pkg/bus`
defect worth filing on its own — the steward watch is already independent of
seq ordering and of the read flag, and must not grow a workaround for it.

Seat-channel detection is therefore **intermittent on this host** for the same
reason: proven once end to end, not reproducible while the seq collisions
persist.

## Known gaps

- **Windows has no graceful stop.** There is no SIGTERM to ask with, so
  `stewardTermSignals()` is empty there and a stop is a kill — the wrap-up does
  not run. Returning `os.Kill` would be worse (uncatchable). A record-file
  signal is the fix.
- **`mediator` is not yet a bus address.** Making it a role alias resolving to
  the steward seat topic would let `bashy mb send --to mediator` reach the
  host's point of contact — one seat, two names — and is a small follow-up.
- **The bus seq collision above.** Until it is fixed, push delivery on this host
  is best-effort; pull (`steward inbox` / `mb`) is unaffected and is what the
  bootstrap brief tells the agent to rely on.
