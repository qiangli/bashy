---
id: 8624c8fab52e
kind: task
title: 'Bashy v1.2.0: durable agentic time orchestration'
seq: 6
status: todo
priority: p2
created: 2026-08-11T17:26:27Z
---

Design and deliver one durable time model for Bashy's classic time commands and
its agent lifecycles. This feature is targeted for the **Bashy v1.2.0 release,
after v1.0.0 certification**: it must not displace the v1/POSIX certification
campaign or be used to weaken an existing compatibility gate.

## Agentic spacetime principle

Design every time-related command, option, and timestamp as the **time
dimension of agentic spacetime**, not merely as a traditional date parser,
timer, or delayed shell invocation. An agentic action has both a spatial
coordinate (host, workspace, target resource, and execution context) and a
temporal coordinate (nominal time, timezone, deadline, duration, recurrence,
and recovery policy). The interface must preserve both coordinates so an agent
can answer: what should happen, where, when, for how long, relative to which
clock, and what should happen if that moment is missed.

Classic flags remain compatible, but their internal model and Bashy extension
surface should compose cleanly with agent plans, durable wakeups, deadlines,
leases, cancellation, and lifecycle transitions. Prefer typed, inspectable
temporal intent over opaque sleeps or embedded command strings. This principle
also complements Bashy's existing space-time advisor: space explains whether
an execution context is viable, while time explains when an action becomes
eligible, late, expired, retryable, or complete.

## Why this is one feature

Bashy already has useful pieces, but not yet one trustworthy clock:

- `schedule` persists `cron`, `every`, and `at` jobs and offers
  `add/list/rm/run/tick/daemon/start/status/stop`;
- `at`, `atq`, and `crontab` share that store, while `timeout`, `sleep`, `cal`,
  and `ncal` are separate compatibility commands;
- `chat`, `meet`, `sprint`, and `weave` each own meaningful lifecycle state;
- the scheduler currently claims a due job before invoking it, but has no
  durable occurrence ledger, execution lease, acknowledgement, retry decision,
  or recovery proof if the process or host dies between claim and completion;
- cron and local `at` parsing do not yet persist an explicit timezone/DST
  decision, and a ticker alone is not a clock-jump or reboot policy.

Do not grow a second scheduler inside each agent feature. `bashy schedule` is the
durable control plane. Classic commands are compatibility adapters or local time
utilities, and agent operations are typed actions dispatched to the packages
that own their state.

## User model

A schedule definition answers four questions: **when**, **in which timezone**,
**which typed action**, and **under whose bounded authority**. Each nominal fire
creates a durable occurrence with its own identity and history.

The action is either:

1. a command invocation with explicit argv/cwd/environment policy and an effect
   ceiling; or
2. a typed lifecycle action targeting a stable `chat`, `meet`, `sprint`, or
   `weave` identity.

The lifecycle vocabulary must map onto the target package's real state machine,
not shell out to `bashy ...` or synthesize prompts through environment variables.
Expose only transitions that the owner can implement coherently: wake/start,
stop, pause, and resume where supported. An already-satisfied idempotent
transition is an observed no-op success; an illegal transition is recorded as a
failure, never silently approximated. In particular, scheduling itself uses
`disable`/`enable` (or equally unambiguous terms), so “pause a schedule” cannot
be confused with “pause the target sprint or weave.”

The command boundaries remain clear:

- `at`/`atq` (and removal through the supported `at` interface) are the POSIX
  one-shot shell-job view; `crontab` is the POSIX recurring-job view.
- `timeout` is the correctly named GNU-compatible deadline wrapper for one
  execution. It does not create a durable job.
- `sleep` is a cancellable delay in the current process. It does not survive a
  restart and must not pretend to be scheduling.
- `cal` and `ncal` display calendars. They may share timezone parsing, but never
  mutate scheduler state.
- `bashy schedule` is the Bashy extension for richer policy, typed actions,
  history, and administration.

## Durable data and execution contract

Version the store and make migrations explicit and fail-closed. Keep definitions
separate from append-only occurrence history (with bounded compaction):

- A **definition** has an immutable job ID, revision, owner/principal, creator,
  schedule expression, schedule kind, IANA timezone, typed action and stable
  target, creation time, enabled state, concurrency policy, misfire policy,
  retry ceiling/backoff, effect/capability ceiling, and redaction metadata.
- An **occurrence ID** is deterministically derived from job ID, definition
  revision, and nominal scheduled instant. It is the deduplication key across
  daemon restarts and clock corrections.
- Occurrences move through `pending`, `claimed`, `running`, and a terminal state
  (`succeeded`, `failed`, `missed`, `cancelled`, or `unknown`). Claims carry an
  expiring lease, attempt number, heartbeat, and fencing token so a stale daemon
  cannot commit over a replacement.
- Record nominal time, actual start/end, lag, result/status, retry decision, and
  a redacted diagnostic. Never place raw secrets, tokens, complete captured
  environments, or private prompts in list/history output.

Use the existing file ownership and atomic-write protections as a baseline, then
add a single-writer lock, versioned journal/checkpoint, directory sync where the
platform requires it, corruption detection, and explicit recovery. A crash after
claim but before acknowledgement becomes `unknown`; do not blindly replay a
possibly side-effecting command. Automatic replay is allowed only when the typed
action is proven idempotent or its declared policy permits it.

Definitions default to `forbid` concurrent occurrences for the same job and
target. Richer `allow`/`replace` policies must be explicit. A caller may supply a
deduplication key, but cannot weaken the occurrence identity or fencing rules.
Wake/steer delivery needs a durable delivery ID and acknowledgement so recovery
cannot duplicate an agent message.

## Clock, timezone, and missed-time semantics

Never infer durable semantics from the daemon's process-global `time.Local`:

- An RFC 3339 one-shot with an offset denotes one absolute instant.
- A local wall-time one-shot persists its wall fields and IANA zone. Reject a
  nonexistent DST time. Reject an ambiguous fold unless the caller explicitly
  selects the first or second occurrence.
- A calendar/cron schedule is evaluated in its persisted IANA zone. A spring
  gap is recorded as missed; a fall fold produces the explicitly documented
  single occurrence, not two accidental runs. The policy must be fixed in the
  schema and visible in `show`.
- `every` means elapsed cadence while the daemon is alive. Persist the next
  nominal instant; after downtime, apply its misfire policy instead of bursting
  all elapsed intervals.

Use monotonic timers only to optimize sleeping. Recompute eligibility from the
wall clock at every wake. A backward clock jump cannot repeat a committed
occurrence because its ID already exists. A forward jump invokes the definition's
misfire policy:

- `skip`: record missed occurrences without running them;
- `run-once`: collapse the missed window into one immediate occurrence (the
  default for idempotent lifecycle wakeups and a sensible recovery option for a
  one-shot job);
- `catch-up`: replay in nominal order, but only with a configured hard count/time
  bound.

There is no unbounded catch-up mode. Persist both nominal and observed timestamps
so NTP adjustments, suspend/resume, and reboot behavior are explainable.

## Authorization and effect bounds

Creation authority is not a bearer token for future unlimited execution. Record
the creator, target scope, requested effects, and maximum capabilities, then
reauthorize every occurrence against current policy. Revocation after creation
must prevent execution. Stored actions reference secrets through the normal
secret mechanism; they never snapshot secret values.

Command jobs require an explicit effect ceiling and constrained cwd/environment
policy. Typed agent actions use the target package API and the least capability
needed for that transition. Provider/model budgets, fan-out, external messaging,
filesystem/network effects, and child-process creation remain subject to the
current atlas/effect checks. Scheduling cannot manufacture grants or extend an
expired grant. Listing a job must be safe for the list caller even when the job's
owner was authorized to see more.

## Service lifecycle and recovery

Provide an explicit per-user service installation path for supported platforms
(for example launchd/systemd and an equivalent Windows mechanism) with one daemon
and one store lock. Do not secretly edit host crontabs. Startup validates the
store, acquires the lock, expires stale leases, applies misfire/recovery policy,
and only then accepts work. Shutdown stops claiming, drains or cancels according
to policy, checkpoints, and releases the lock. Reboot tests must prove that a
definition survives, an occurrence is neither lost nor duplicated, and corrupt
state fails closed without overwriting the last good checkpoint.

## Observation and control

Add stable human and JSON views for definition list/show, occurrence history,
scheduler status, next nominal/effective fire, timezone/DST choice, lease/attempt,
last result, misfires, retries, and disabled reason. Provide separate operations
to remove future definitions, disable/enable them, cancel a pending occurrence,
and request cancellation of a running occurrence. Running-command cancellation
must reach its process tree; typed-action cancellation must use its owning API.
Cancellation preserves the occurrence record.

Emit structured audit events and metrics for schedule lag, due/claimed/running
counts, missed fires, stale leases, retries, cancellations, denied effects, and
duplicates prevented. Logs and traces carry IDs, not unredacted argv, prompts,
contexts, environments, or secret material.

## Compatibility boundary

Keep POSIX `at`, `atq`, and `crontab` parsing, output, environment/umask capture,
queue behavior, status, and diagnostics as compatibility contracts. Their public
views must not leak Bashy-only metadata. Preserve GNU-compatible `timeout` and
`sleep` options and exit-status behavior. Preserve `cal`/`ncal` display behavior.
Any richer timezone, history, retry, or lifecycle control belongs to
`bashy schedule` and makes no POSIX/GNU certification claim. Never use the new
engine as justification to accept a classic-command regression.

## Delivery phases

1. **Contract:** write the versioned schema, occurrence state machine, ownership
   and effect model, lifecycle transition table, compatibility matrix, and
   migration/rollback design before changing execution.
2. **Ledger and recovery:** add occurrence IDs, leases/fencing, history, locking,
   atomic journal/checkpoint recovery, and read-only observability while command
   behavior remains otherwise unchanged.
3. **Time engine:** add injected clocks, persisted IANA zones, DST decisions,
   clock-jump handling, bounded misfire policies, and deterministic next-fire
   calculation.
4. **Classic adapters:** route `at`/`atq`/`crontab` through the durable engine
   without changing their compatibility surface; explicitly verify that
   `timeout`, `sleep`, `cal`, and `ncal` remain non-durable utilities.
5. **Typed actions:** introduce a small action registry and package-owned adapters
   for chat/meet/sprint/weave wake/start/stop/pause/resume, with acknowledgements,
   idempotency, cancellation, and reauthorization.
6. **Operations:** add per-user service installation, status/history/cancellation,
   redacted telemetry, migrations, retention, and operator runbooks.
7. **Hardening:** complete cross-platform, race, crash, reboot, long-soak, and
   compatibility gates before enabling automatic service startup by default.

## Required tests and acceptance gates

- Pure fake-clock tests cover forward/backward jumps, suspend, leap day, month
  boundaries, spring gaps, fall folds, explicit fold selection, and zones with
  non-hour offsets. Tests do not modify process-global timezone state.
- Crash injection covers every boundary before/after persist, claim, dispatch,
  acknowledgement, terminal commit, checkpoint, and compaction. Torn/corrupt
  stores, two daemons, expired leases, stale fencing tokens, and interrupted
  migration fail closed.
- Restart/reboot matrices prove each `skip`, `run-once`, and bounded `catch-up`
  policy. A backward jump and repeated startup never duplicate an occurrence.
- Concurrency tests prove per-job/target exclusion and race-clean immutable
  definitions. Delivery tests prove one wake/message for one delivery ID.
- Lifecycle tests exercise every legal and illegal transition, already-satisfied
  no-ops, cancellation, and the distinct schedule-disable versus target-pause
  meanings for chat, meet, sprint, and weave.
- Security tests revoke authority after creation, lower an effect cap, deny target
  access, rotate a secret reference, and inspect every human/JSON/log surface for
  leakage. Stored authority cannot mint or prolong a grant.
- Compatibility tests retain the complete existing `at`/`atq`/`crontab`,
  `timeout`, `sleep`, `cal`, and `ncal` suites and affected POSIX/GNU verifier
  gates. New extension tests are separate from conformance evidence.
- Service tests use isolated launchd/systemd/Windows adapters, prove one active
  daemon, clean shutdown and restart, and leave no real host service or crontab
  behind. Run focused, full, race, cross-platform build/vet, and store migration
  tests.

The feature is complete only when a scheduled typed lifecycle action can survive
a daemon and host restart, execute once under still-valid bounded authority, be
cancelled and audited without secret leakage, and coexist with unchanged classic
command compatibility.

## Non-goals

- A distributed consensus scheduler or exactly-once arbitrary shell side effects.
- A calendar UI, hosted SaaS dependency, or hidden host-cron integration.
- Keeping a provider session alive merely because a future action exists.
- Inferring lifecycle state by scraping CLI output or replaying private prompts.
- Expanding pre-v1 certification scope to deliver this v1.2.0 feature.
