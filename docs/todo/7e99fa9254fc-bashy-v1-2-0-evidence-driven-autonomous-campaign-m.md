---
id: 7e99fa9254fc
kind: task
title: 'Bashy v1.2.0: evidence-driven autonomous campaign management'
seq: 7
status: todo
priority: p1
created: 2026-08-11T19:06:17.417403Z
---

Turn the lived POSIX certification campaign’s management failures into a
bounded, observable, evidence-driven, and self-healing control loop for
stewards, conductors, and workers. Target **Bashy v1.2.0, after v1.0.0**. This
must not interrupt certification or weaken its acceptance gate.

## Retrospective: what repeatedly failed

The campaign proved that launching more agents is easy; maintaining truthful
ownership and converting their work into verified delivery is hard.

- Sprint status reported “no live conductor” while conductor processes or
  leases existed, and at other times a refreshed lease hid a wedged conductor.
- `bashy agents` omitted conductors or showed a replacement process under the
  previous worker’s owner name. Process, queue, lease, room, and launch-spec
  truth were read independently instead of reconciled once.
- Conductors stopped supervising after their initial fan-out. Completed,
  failed, quota-blocked, and killed workers were not promptly replaced, leaving
  expensive capacity idle while the board still looked busy.
- Workers could loop indefinitely because Fibonacci points described size but
  did not initially bind a watchdog. “Working” meant neither recent progress
  nor a deadline.
- A small CLI-ordering mistake—putting weave flags after `--`—provisioned a
  workspace and then tried to execute an agent nickname as a binary. The error
  arrived late and seven launches failed at once.
- Review work was repeatedly wasted because isolated reviewers could not reach
  candidate commits living only in sibling worktrees. Some agents produced
  prose reviews as commits while the actual candidate remained unavailable.
- Large batches accumulated many locally plausible fixes but delayed the only
  measurement that matters: affected Bashy versus retained GNU evidence. This
  turned native validation into an end-of-sprint cliff.
- Counts blurred distinct progress measures: Bashy-only identities, failing
  utilities, executed TPs, and assertions within a TP. A real assertion fix
  could look like no progress, while a passing local test could look too much
  like retirement.
- Agent exit zero was repeatedly treated as delivery evidence even though
  several harnesses exit zero without a usable commit. Conversely, extra judge
  agents were used where deterministic tests and provenance checks were enough.
- Quota failures, read-only Git sandboxes, missing dependencies, and unavailable
  model bindings were discovered after allocation instead of during preflight.
- The retained native droplet sometimes sat idle and billed while candidates
  accumulated; at other times a run was proposed before a clean pushed pin and
  phase-approved boundary existed.

These are product defects, not merely operating mistakes. The best tool for
agents must make the truthful path the easiest path and make ambiguous or
unbounded states structurally impossible.

## Product goal

Provide one campaign manager over the existing `sprint`, `weave`, `agents`,
`room`, `bus`, `dag`, `verify`, and scheduling primitives. It does not introduce
a fourth orchestration role. The three seats remain:

1. **Steward:** owns campaign scope, acceptance evidence, expensive/native
   resources, integration, and release truth.
2. **Conductor:** owns one sprint lease, prioritizes whole-tool work, supervises
   its workers continuously, and replaces or decomposes stalled work.
3. **Worker:** owns one bounded run and returns code/evidence or an explicit
   decomposition/blocked result.

The manager should automate reconciliation and policy, not judgment. Tests are
the default approver. A second model is requested only for ambiguous semantics,
security/trust boundaries, destructive lifecycle changes, or failures without
a deterministic discriminator.

## Required capabilities

### 1. One reconciled live roster

Build a single snapshot joining sprint lease, weave queue, launch spec, wrapper
PID, process liveness, room membership, control socket, heartbeat, workspace,
and agent catalog identity. Every human and JSON view consumes that snapshot.

- `bashy agents` must show the steward/conductor/worker hierarchy, including
  live conductors, actual replacement owners, assigned run, points, deadline,
  last progress, and health reason.
- `bashy sprint status` must distinguish `healthy`, `stale`, `wedged`,
  `unowned`, `draining`, and `closed`; a fresh lease alone is not health.
- Conflicting evidence is an explicit `inconsistent` state with repair advice,
  never a plausible green state.
- Stale ownership is reconciled atomically when a run is reassigned. Historical
  owners remain in events, not in the live owner field.

### 2. Bounded work as an invariant

Build on the shipped Fibonacci runtime caps. Every sprint-linked run must have
points in `1,2,3,5,8`, a persisted finite runtime, an idle/progress policy, and
a defined terminal artifact. Reject linking or launch before provisioning when
any is absent or contradictory.

- Changing points after claim is forbidden; stop/replan/restart instead.
- At 50% and 80% of budget, require lightweight progress evidence: changed
  files, a test result, a committed checkpoint, or a structured diagnosis.
- At the cap, terminate the process tree, preserve the workspace, classify the
  result, and immediately notify the conductor. No automatic time extensions.
- Detect repetitive no-progress tool loops cheaply (total operations versus
  distinct operations, repeated identical errors/reads) and interrupt before
  the wall-clock cap.
- A blocked worker must state the smallest next packet and what evidence would
  unblock it. “Still trying” is not a checkpoint.

### 3. Proactive conductor supervision

Make supervision a durable loop rather than a prompt convention. A conductor
declares target concurrency and WIP limits, then receives change-driven events
when workers start, checkpoint, submit, fail, go idle, exceed budget, or lose
their process.

- Keep useful slots filled while backlog remains, subject to WIP and resource
  caps. Default to a small number of whole tools—approximately four active
  semantic packets per conductor—even when more low-band agents are available.
- Replace a failed worker only after classifying the failure: task defect,
  environment, quota, permission, missing candidate, or model/tool mismatch.
- Rescope after one failed bounded attempt when the next attempt would repeat
  the same plan. Do not spend another budget merely changing the agent name.
- Escalate a stale or wedged conductor to the steward automatically, preserving
  its checkpoint and worker workspaces. The steward can replace the conductor
  without manually rebuilding the roster.
- Conductors periodically commit source-ready work; uncommitted WIP receives a
  preservation checkpoint before reassignment or cleanup.

### 4. Whole-tool campaign planning

The planning unit is a tool or a tightly coupled command family, not an opaque
blocker number. A tool packet carries:

- exact current Bashy-only identity inventory and target TP denominator;
- public reproducer red on the pushed base;
- POSIX/GNU requirement and local reference source location/function when
  legally usable as a semantic reference;
- bounded source scope and known adjacent interactions;
- focused, race, matrix, cross-platform, and full-scope gates appropriate to
  the change;
- affected native target(s) and expected evidence comparison.

Prioritize small-denominator and high-confidence tools, but keep batching
economic: a one-identity fix is progress only when its integration and native
measurement cost is proportionate. Support configurable lanes for low-hanging
long-tail work without mixing their files or ownership.

### 5. Evidence-first delivery pipeline

Represent delivery as a machine-checkable chain:

`public red → committed patch → deterministic local gates → pushed component →
consumer pin → pushed umbrella approval → native affected arm → retained GNU
subtraction → roadmap arithmetic`.

- A candidate is not “submitted” without a reachable commit SHA, clean status,
  base SHA, changed-file allowlist, and gate manifest.
- Review workspaces receive candidate objects through an explicit immutable
  bundle/ref. Never ask a reviewer to infer another worktree path.
- Let the steward approve automatically when required tests, diff constraints,
  provenance, and public oracle all pass. Record why an agent judge was needed
  when one is used.
- A harness exit code alone is never evidence. The declared gate and its
  expected artifacts determine success.
- Native retirement is computed mechanically from exact Bashy and retained GNU
  identities. No TP number is assigned a semantic meaning without permitted
  evidence.

### 6. Multilevel progress accounting

Expose four non-interchangeable counters per tool, sprint, and campaign:

1. Bashy-only identity count;
2. utilities/CUs with remaining Bashy-only failures;
3. executed TP outcomes and denominator;
4. known assertion/category defects within still-failing TPs.

Also report source-ready, integrated, native-pending, evidence-retired, and
rejected/held candidate counts. Historical baseline, current exact evidence,
and provisional planning estimates must be labeled separately. Never turn the
absence of a current result into zero.

### 7. Preflight before allocation

Before cloning or changing queue state, verify:

- registered agent/tool/model resolution and headless launch argv;
- quota/auth availability with a cheap bounded probe;
- Git write/commit capability and candidate/base reachability;
- required dependency/submodule hydration;
- points, runtime, workspace path, file ownership, and non-overlap;
- placement of weave flags versus raw argv.

Return a structured failure with zero workspace/owner/queue mutation. Offer a
machine-readable corrected invocation. Keep explicit raw argv supported; do not
silently reinterpret it as an agent launch.

### 8. Native resource economics

Treat a paid droplet as a leased test resource with an owner, purpose, expected
targets, start deadline, and idle policy.

- Dispatch only from a clean, pushed, phase-approved component/pin boundary.
- Build once, run the smallest affected targets sequentially under one native
  lock, retrieve each result immediately, and update counts immediately.
- Surface billed-idle duration and warn/escalate when an approved host has no
  queued arm. Optionally stop an explicitly campaign-owned droplet after a
  configurable idle grace period, but never destroy or stop external resources
  without an authority record.
- Preserve diagnostic versus certification provenance; targeted arms never
  become stitchable acceptance evidence by wording.

### 9. Safe launch grammar

Make agent selection structurally distinct from raw command execution. Prefer
an explicit `--agent <identity>`/`--tool <binding>` typed field in persisted
launch specs, while retaining `-- <raw argv...>` as exactly raw. The CLI must
detect known weave flags misplaced after `--` before provisioning and explain
the corrected order. Shell completion and generated examples must never emit
the ambiguous form.

### 10. Durable event-driven manager

Use the existing append-only event/bus foundation and the v1.2.0 agentic-time
work to wake managers on state changes and deadlines. Avoid a polling-only
daemon and avoid a second scheduler.

- Every transition has an event ID, actor/principal, prior/new state, reason,
  evidence pointer, and timestamp.
- Checkpoints and assignments are idempotent; replacements use fencing so a
  stale conductor cannot overwrite its successor.
- On restart, reconstruct current state from durable records and reconcile live
  processes before acting.
- Notifications demote rather than disappear when immediate delivery fails.

## User experience

Add concise views rather than more unrelated verbs:

- `bashy sprint status`: campaign summary, conductor health, WIP, deadlines,
  evidence stage, exact progress counters, and next expensive action.
- `bashy agents`: complete live hierarchy plus health and deadline.
- `bashy weave doctor --sprint N`: reconcile and explain inconsistencies,
  missing commits, unreachable candidates, stale processes, and budget drift.
- `bashy sprint watch N`: change-driven supervision stream suitable for humans
  and conductor sidecars.
- Stable JSON schemas for every view; terminal prose is a rendering of the same
  snapshot, not separately computed truth.

Prefer enhancing these existing nouns over adding `campaign-manager`,
`orchestrate`, or another overlapping command.

## Safety and compatibility

- Keep `cmd/bash` free of AgentOS code and preserve classic command behavior.
- Never expose licensed journal contents or proprietary harness internals
  through OSS status/events.
- Never copy GNU GPL source; local GNU/Bash source is semantic reference only.
- Never let automation push, merge, mutate roadmap counts, access native
  resources, or stop infrastructure beyond its explicit authority record.
- Preserve dirty/untracked user files and unrelated submodule changes.
- No success state may be inferred from missing evidence, silence, an empty
  roster, or process exit zero.

## Delivery plan

1. **Truth model:** define the reconciled snapshot, health states, event schema,
   evidence stages, and four progress counters.
2. **Preflight and bounded execution:** finish point/runtime enforcement,
   typed launch parsing, capability/quota/Git probes, and no-mutation failures.
3. **Roster and status:** make `agents` and `sprint status` consume one snapshot;
   add inconsistency diagnostics and complete conductor visibility.
4. **Supervision loop:** event-driven conductor wakeups, progress thresholds,
   stall classification, replacement/rescope, and steward escalation.
5. **Candidate/evidence chain:** reachable commit bundles, gate manifests,
   test-first approval policy, affected-native queue, retrieval, and mechanical
   retained-GNU subtraction.
6. **Resource economics:** droplet lease/idle accounting and authorized
   lifecycle policy.
7. **Campaign replay:** run a bounded public fixture campaign with injected
   conductor death, worker quota failure, missing commit, stale owner,
   flags-after-`--`, test failure, native-host delay, and restart recovery.

## Acceptance criteria

- A 10-worker synthetic sprint can lose its conductor and three workers, then
  recover automatically without duplicate ownership, lost workspaces, or an
  unbounded process.
- Every linked run has valid points and a finite watchdog; invalid, over-limit,
  or post-claim point changes fail before mutation.
- Roster and sprint JSON agree byte-for-byte on live owners and states under
  concurrent replacement and process death.
- A submitted candidate is reachable from a fresh isolated reviewer checkout
  and carries reproducible local gate results.
- Deterministic green gates approve an ordinary low-risk patch without a judge
  agent; an ambiguous semantic fixture records the explicit reason for review.
- A fake native campaign proves build-once, affected-target dispatch,
  immediate retrieval, exact subtraction, and no retirement on missing or
  mismatched provenance.
- A restart during allocation, execution, review, and native retrieval is
  replay-safe and does not turn silence into success.
- The existing Bash 5.3, POSIX campaign, cross-platform, race, and orchestration
  gates remain green.

## Non-goals

- Replacing human/steward release authority.
- Making model output itself trusted evidence.
- Automatically guessing opaque TP semantics.
- Unlimited concurrency or automatic deadline extensions.
- A new role, scheduler, tracker, chat system, or orchestration noun.
- Bundling certification material or proprietary harness details into Bashy.
