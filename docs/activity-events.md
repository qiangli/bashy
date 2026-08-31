# The activity-event contract

**Status:** shipped (bashy#12). Package `internal/agentos/activity`; control
surface `bashy activity`; documentation of record for the envelope, the
interest-routing matrix, the delivery guarantees, and the adapter API that
bashy#11 consumes.

## The problem

Every Bashy-integrated agent needs to know about the system activity that
concerns it, and the two obvious answers are both wrong.

**Polling** costs tokens on every turn to learn, almost always, that nothing
happened. An agent that runs `bashy inbox`, `bashy mb`, `bashy todo list` and
`bashy weave status` at the top of each turn has spent a real fraction of its
context before it starts thinking, and the fraction grows with the fleet.

**Broadcast** costs attention. An agent interrupted for something irrelevant is
worse off than one left alone, and an inbox that is nine parts noise gets
skimmed — which is the same as not delivering the tenth part.

So: **push, but only to interested parties, and say why.**

## What already existed, and what this adds

bashy#10 shipped the transport. This contract adds no second one:

| primitive | what it is | who owns it |
|---|---|---|
| `bus.Publish` | durable append to the append-only room timeline | bashy#10 |
| `bus.EnsureSubscription` | give an offline identity a durable inbox | bashy#10 |
| `bus.SteerLive` | wake a live agent over its session control socket | bashy#10 |
| `bashy inbox` | the one receive-side view | `docs/unified-inbox.md` |
| **`activity.Emit`** | **the envelope, the routing, and the ordering** | **this** |

**There is no second mailbox.** A recipient reads its activity in `bashy
inbox`, next to everything else addressed to it. The only state this package
keeps is an **outbox** — an emit-side journal whose entire job is dedup,
per-source sequencing and crash recovery. It holds no per-recipient read
state, because pkg/bus already does and two answers to "have I read this" is
worse than none.

This also replaces three hand-rolled versions of the same idea:
`pkg/weave/weave_notice.go`, `pkg/kb/bus.go` and `pkg/chat/coach.go` each
independently made the same five decisions (dedup key, ordering, durable-first,
wake, offline inbox), and three spellings of one idea drift invisibly.

## The envelope

Schema `bashy-activity-v1`. Defined in `internal/agentos/activity/event.go`.

| field | meaning |
|---|---|
| `schema`, `id`, `seq`, `at` | assigned by `Emit`, never by the caller |
| `source` | the subsystem: `mb meet weave sprint todo inbox ping notify` |
| `actor` | who did it |
| `action` | `create read update delete` + `start finish fail cancel block` |
| `noun` | the object kind (`run`, `story`, `task`, `message`, …) |
| `object` | the stable object reference (`weave:run/42`) |
| `scope` | `repo` / `sprint` / `topic` / `room` |
| `status` | `ok failed blocked pending` — minimal, and optional |
| `mentions`, `owner`, `assignees`, `depends_on`, `members` | routing inputs |
| `cause`, `hop` | the loop-prevention chain |
| `token` | the caller's transaction-boundary discriminator |

### The envelope cannot carry a body

The requirement is that an activity event never carries full bodies, prompts,
secrets, terminal output or diffs. That is enforced **structurally**, not by
review:

- **There is no body field.** `Event` has nowhere to put a diff. A policed rule
  holds until somebody is in a hurry; an absent field holds forever.
  `TestEnvelopeHasNoBodyField` fails the build if one is added.
- Every string is capped at **96 bytes** (64 for identities), and lists at 32
  entries. Refused, never truncated — a silently shortened object reference is
  a pointer that resolves to nothing.
- **Control characters are refused**, which is what actually excludes terminal
  output and diffs.
- `action` and `status` are **closed vocabularies**. An open action vocabulary
  is an open body field with extra steps.
- Defence in depth: a value containing a known credential prefix (`ghp_`,
  `sk-`, `AKIA`, `-----BEGIN`, …) is refused by name.

### The event id is stable and time-free

`id = sha256(source, actor, action, noun, object, scope, token)[:20]`.

Neither `at` nor `seq` participates. Putting a clock in a key is the mistake
`docs/agentic-history-and-space-graph.md` records the cost of: every emit
becomes unique, the dedup index fills with n=1 singletons, and at-least-once
quietly degrades to once-**per-retry**. What makes two emits the same event is
that they describe the same actor doing the same thing to the same object at
the same transaction boundary — which is exactly the tuple hashed.

`token` is what distinguishes this commit of an object from the next: a
terminal state, a revision, a monotonic id. **A caller that passes a timestamp
has defeated the dedup.**

### Rendering

`Event.Subject()` is the compact one-line form a recipient sees:

```
weave fail run weave:run/42 failed [repo=bashy sprint=88] id=6a3f…
```

Verb-first, no ceremony, scope only when set, bounded at 256 bytes. If the line
would exceed that, **scope fields are elided** in a fixed order — never the
object, and never the trailing `id=`. Eliding a rendering is safe because the
rendering is a view: `bashy activity show <id>` returns the complete envelope.
That is why the id is the one field that must survive.

## The interest-routing matrix

An event reaches an identity only when a **named relationship** connects them,
and the relationship is reported with the delivery. A recipient that cannot see
why it was told something cannot judge whether the sender was wrong.

### Precedence — strongest first

| # | reason | matches when | wakes? |
|---|---|---|---|
| 1 | `mention` | the identity is in `event.mentions` | ✅ |
| 2 | `assignment` | the identity is in `event.assignees` | ✅ |
| 3 | `ownership` | the identity is `event.owner` | ✅ |
| 4 | `subscription` | a declared `Interest` matches the scope, object, source or noun | queued |
| 5 | `dependency` | a declared `Interest` watches an object in `event.depends_on` | queued |
| 6 | `membership` | the identity is in `event.members` | queued |

The order is **directness of this event to this recipient**, not how the
relationship was established. A standing subscription says "tell me about
things like this"; a mention says "this one is about you" — when both are true
the second is the honest explanation. Membership is last because it is the only
reason that is true of a whole group rather than a person.

A recipient matching several reasons is reported under the **highest**
one. Output is sorted by (precedence, subscriber), so it is deterministic: two
agents always agree about who was told and why.

### The rules that bind regardless of reason

- **The actor is never a recipient.** An agent does not need to be told what it
  just did, and an actor-echo is the cheapest way to build an accidental loop
  out of two subsystems that each react to the other.
- **Declared filters bind on every reason.** An `Interest` that says
  `--source weave` means it, even when the match came from a mention.
- **`Mute` suppresses routing entirely.** This is the one place an event is
  deliberately not delivered, and it is an instruction from the subscriber
  rather than an inference by the router.
- **`Wake: false` outranks even the strongest reason.** An operator who decided
  this agent is never interrupted has expressed a policy.

### Read events are never broad-broadcast

A `read` event reaches **nobody** unless an identity asked for read activity by
name — `Interest.Audit`, or `read` listed explicitly in `Interest.Actions`. It
never routes on the membership axis, because membership is the broadcast axis
wearing a different hat. On a host with no audit interest declared,
`Adapter.Read` is a journal entry and nothing else. That is the intended
behaviour, not a misconfiguration.

### Loop prevention

Three floors, all tested:

1. Source `activity` is **reserved** and routes to nobody. Delivering an event
   necessarily touches the bus; if bus traffic could itself be announced as
   activity, the first event would be the last thing the host ever did.
2. `Emit` **refuses** a reserved-source event outright, so the refusal lands at
   a named call site.
3. `Adapter.Reacting` carries `cause`/`hop` forward and `Validate` refuses at
   `MaxHop` (3). A cycle terminates in a reported error, not an unbounded
   fan-out.

Plus the actor exclusion above, which breaks the two-subsystem case.

## Delivery guarantees

`Emit` runs this order, and the order **is** the guarantee:

```
1. journal the record as UNDELIVERED, fsync      ← the crash evidence
2. bus.EnsureSubscription(recipient)             ← offline recipients
3. bus.Publish(...)                              ← THE DURABLE APPEND
4. journal the per-recipient delivery            ← dedup on replay
5. bus.SteerLive(...)                            ← THE WAKE
```

| guarantee | how |
|---|---|
| **at-least-once** | step 1 precedes step 3, so a crash leaves evidence a delivery is owed; `bashy activity recover` re-drives it |
| **stable event ids** | the time-free hash above |
| **deduplication** | a fully-delivered id makes `Emit` a no-op returning `duplicate: true` — safe to call from any recovery path |
| **ordering per source** | `seq` is assigned per source under a kernel lock. *Per source, not global*: a global counter would imply a total order across subsystems that no single lock establishes, and an implied guarantee is the kind relied on exactly once, in the incident |
| **reconnect catch-up** | `activity.Since(source, seq)` answers "what did I miss", asked once on reconnect |
| **no lost event between durable append and wake** | the wake is strictly downstream of the append and never gates it. A failed wake costs latency — the recipient reads the same event from the inbox step 3 already wrote. A wake that came first, or a publish skipped because a wake succeeded, would cost the event |
| **partial fan-out resumes** | `Delivered` is per recipient, so a crash midway through a twelve-recipient fan-out resumes at the thirteenth rather than choosing between re-publishing to all or none. One unreachable agent never silences the rest of the fleet |

### Backpressure and coalescing: DEMOTE, NEVER DROP

Two controls, and **both apply to the wake only**:

- **Coalescing** — one wake per `(recipient, source, object)` per 10s window.
- **Rate limiting** — `MaxWakePerMin` (default 3, mirroring
  `bus.DefaultMaxPerMin` and its reasoning) wakes per recipient per minute.

Every routed event is still **durably published, every time**. Suppressing the
durable copy would be the tidier implementation and would silently drop the
second half of a rapid create-then-fail pair — the half that mattered. What is
lost is the interruption, which is the thing that was too frequent. Wake
outcomes (`steered queued coalesced rate-limited unreachable`) are recorded per
recipient and reported by `bashy activity status`.

**A wake outcome is never a delivery outcome.** `queued` and `unreachable` both
sit on top of a successful durable append.

### Polling is the fallback, not the path

`activity.Tail` and `bashy activity tail` exist for an operator diagnosing
delivery and for a subscriber closing a gap it cannot close from its own
cursor. An agent that polls `Tail` on a timer has reintroduced exactly the
token-heavy polling this contract removes.

### Retention

The journal keeps the newest `MaxJournalRecords` (5000) folded records and
**always keeps every unpublished one regardless of age** — an unpublished
record is an owed delivery, and dropping it would convert at-least-once into
sometimes. A dedup key older than the window can publish a second time; that is
exactly the at-least-once semantic claimed here, so pruning weakens nothing,
and `bashy activity show` says so by name when an id has been pruned.

## The adapter API (consumed by bashy#11)

The constraint is that wiring a subsystem must be a **one-line call at its
transaction boundary**. Anything longer and the wiring drifts per subsystem,
which is the failure this package exists to end.

```go
import "github.com/qiangli/bashy/internal/agentos/activity"

a, err := activity.For(activity.SourceWeave)          // refuses an unregistered source
a = a.As("conductor").In(activity.Scope{Repo: "bashy", Sprint: "88"})

// ... the subsystem's own write COMMITS here ...

a.Lifecycle(activity.ActionFail, "run", "weave:run/42",
    activity.StatusFailed, /*token*/ it.State,
    activity.Interested{
        Owner:     owner,
        Assignees: []string{it.Worker},
        Members:   sprintMembers,
    })
```

Surface:

| call | use |
|---|---|
| `For(source)` | bind a subsystem; refuses an unregistered or reserved source |
| `.As(actor)` / `.In(scope)` | bind the constants (both return copies) |
| `.Created/.Updated/.Deleted(noun, object, token, who)` | the CRUD shorthands |
| `.Read(noun, object, token, who)` | separate on purpose — a read is never broad-broadcast |
| `.Lifecycle(action, noun, object, status, token, who)` | start/finish/fail/cancel/block |
| `.Reacting(cause, …)` | an event caused by another; carries the loop chain |
| `.Announce(…)` | the general form the rest delegate to |
| `RegisterSource(name)` | admit a new subsystem, so `bashy activity sources` can enumerate it |

### Rules for an adapter author

1. **Emit at a SUCCESSFUL transaction boundary — after the write commits.**
   Emitting before it announces a fact that may not become true, and an agent
   woken to look at a run that does not exist learns to distrust the channel.
2. **`token` must not be a clock.** See the id section.
3. **Emission must not fail the transaction.** Treat the error as advisory
   (`_, _ = a.Updated(...)`) — the subsystem's own write already committed, and
   an undelivered notice is recoverable while a rolled-back commit is not.
4. **Supply the routing inputs you actually know.** An event with none of them
   routes only to standing interests matching its scope, which is the correct
   floor for a change that concerns nobody in particular. Do **not** populate
   `Members` with "everybody" — that is broadcast, re-entering by the back door.
5. **No CLI emit exists, deliberately.** `bashy activity` has no `emit`
   subcommand: emission is a call at a transaction boundary, and a CLI emit
   would let any caller forge an activity fact with no committed write behind
   it.

## The control surface

`bashy activity` is a control surface, **not a second inbox**:

```
bashy activity status              delivery health: journal size, owed, wake outcomes
bashy activity subscribe <who>     --source --noun --action --repo --sprint --topic
                                   --object --audit --no-wake --mute --max-wake-per-min
bashy activity unsubscribe <who>
bashy activity interests           declared interests, one compact line each
bashy activity tail [N]            recent events (compat/recovery fallback)
bashy activity show <id>           the full envelope behind a rendered `id=`
bashy activity routes <id>         who it reached and WHY each of them matched
bashy activity recover             re-drive any delivery the journal shows as owed
bashy activity sources             the registered subsystem sources
```

`subscribe` **replaces** rather than merges. A merge would make an interest only
ever widen, and the failure mode of this whole design is an interest that
quietly grew into a firehose.

`Interest.Wake` defaults on, unlike `bus.Subscription.InterruptFrom` which
defaults to nobody. These govern different things and both apply: this flag says
whether a *match* may wake, while the bus still refuses an interrupt from a
principal the subscriber has not named. **A wake this verb permits is not a wake
the bus grants** — the governance boundary stays where pkg/bus put it.

## State and isolation

Ladder, matching audit / foreman / the skills store:

```
$BASHY_ACTIVITY_DIR    the specific override, wins
$BASHY_HOME/activity   the whole bashy home relocated
~/.config/bashy/activity
```

**Environment variables do not isolate a test of this package.** `room.Dir()`
reads `BASHY_ROOM_DIR`, *not* `BASHY_HOME`, so a harness that set only the
activity variables and let `Emit` reach the real `bus.Publish` would append to
the operator's live room timeline and steer their live sessions **while looking
hermetic** — a green suite built on data it did not create. The tests therefore
replace the three transport seams (`EnsureInbox`, `PublishDurable`, `WakeLive`)
outright. `TestTransportMustBeStubbedNotRedirected` is the ratchet that says why
the harness cannot be simplified back to environment variables.

## Scope boundary

Shipped here (bashy#12): the envelope, the routing matrix, the delivery path,
the outbox, the control surface, the docs, and the adapter API.

Not here: the per-subsystem call sites. Wiring `mb`/`meet`/`weave`/`sprint`/
`todo`/`inbox`/`ping`/`notify` to call `Adapter.Announce` at their transaction
boundaries is bashy#11's work against the API above — and those subsystems live
in `../coreutils`, a sibling shared by every concurrent workspace, so the
wiring belongs in a coordinated coreutils change with its own `.sibling-pins`
bump rather than in this one.

`pkg/weave/weave_notice.go` is the closest prior art and the first candidate to
migrate: it already does journal-before-publish, stable-key dedup,
`EnsureSubscription` and `SteerLive` by hand. Migrating it is a deletion.
