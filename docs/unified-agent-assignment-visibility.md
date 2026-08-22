# Unified agent assignment visibility

**Status:** implemented, external-orchestrator correction 2026-08-22.

## Problem

`bashy agents` reconciled sprint leases and weave queues, but ignored live room
members that were not attached to a weave run. More importantly, one-shot
`bashy invoke` work deliberately did not join the room. A conductor could
therefore delegate an expensive review to a named or ad-hoc agent and
immediately receive `LIVE 0` from the command intended to answer who was busy.
That makes fair routing impossible and turns durable weave use into an
observability workaround.

This is a product defect. Assignment visibility must be a property of the
launcher, not a convention every caller remembers.

## Contract

1. Every live Bashy-managed agent launch publishes a room card before its child
   process starts and removes that card when the work ends.
2. Interactive sessions remain keyed by agent identity and preserve the
   singleton-agent rule. One-shot work is keyed by the invocation, so it does
   not claim a steerable conversation seat and concurrent ad-hoc work cannot
   collide.
3. `bashy agents` is the canonical unified live-work query. It projects:
   sprint conductor leases, weave runs, and every otherwise-unrepresented room
   member. A room card already represented by a weave item appears once.
4. Named agents use their nickname; ad-hoc agents use the resolved
   `tool:model` binding. The record exposes mode, source, binding, PID, owner,
   repository/cwd label, task, age, and health so a human or managing agent can
   make a workload decision without consulting another command.
5. Human and JSON output share one reconciled snapshot. `bashy agents --json`
   is the stable machine surface; fields are additive to `bashy-agents-v1`.
6. `bashy agents list` remains the fleet catalog. Catalog membership answers
   who *could* work; `bashy agents` answers who *is* working.
7. If publication fails, the launch fails before starting the child: invisible
   work is not allowed. Dry-runs publish no work. Failed or completed launches leave no live card;
   their join/leave remains available from the room timeline.
8. A raw Claude/Codex/etc. parent process is **presence**, not proof of assigned
   work. It is hidden from the default live-work view and remains auditable with
   `--all`; otherwise a week-old idle CLI daemon appears as a busy ghost.
9. An orchestrator that launches outside Bashy's process tree must publish an
   external assignment with `agents track start`, renew its bounded lease, and
   retire it with `agents track stop`. The card is owned by the long-lived
   manager PID, so a different manager cannot overwrite or stop it. An expired
   heartbeat is stale and hidden by default even if the manager remains alive.

The operational surface is deliberately small:

```sh
bashy agents                              # one human snapshot
bashy agents --json                       # one machine snapshot
bashy watch -n 2 bashy agents             # continuously track workload
bashy invoke timeline                     # launch/leave history
bashy agents list --all                    # capability catalog, not occupancy

# External orchestrator bridge (Codex collaboration, MCP, hosted schedulers):
bashy agents track start posix-review --agent Kepler \
  --binding codex:gpt-5.6-sol --role reviewer \
  --task 'review durable POSIX launcher' --owner-pid "$MANAGER_PID"
bashy agents track heartbeat posix-review --owner-pid "$MANAGER_PID"
bashy agents track stop posix-review --owner-pid "$MANAGER_PID"
```

`assignment-id` accepts only letters, digits, dot, underscore, and dash. The
default lease is 30 minutes. Managing agents should heartbeat at ordinary
status checkpoints and use a safe one-line `--task`; the full prompt is never
stored in the roster.

## Task labels and privacy

A live assignment needs a useful work label, but an instruction can contain
arbitrary content. The launcher records only a bounded, single-line summary in
the local 0600 room card. Callers may supply an explicit task label for a
sensitive or very long prompt. The full prompt is never copied into assignment
state.

## Implementation split

- `coreutils/pkg/chat`: publish/remove a work-keyed room card around every
  non-dry-run one-shot invocation; test visibility while a blocking fake runner
  is active and cleanup on every exit path.
- `bashy/internal/agentos/agents.go`: reconcile unmatched room cards, enrich
  weave assignments from their matching card, deduplicate, and expose the
  additional attribution fields in both output modes; provide the explicit
  external-orchestrator tracking bridge and classify raw shell presence as
  idle rather than assigned work.
- `coreutils/pkg/room`: only additive card fields or small helpers needed by
  the launch contract; preserve assignment start separately from heartbeat,
  retain a bounded external lease, and allow a short-lived publisher to retire
  a card owned by its proven manager PID. No new store is introduced.

## Acceptance gates

- A blocked named one-shot and a blocked ad-hoc one-shot both appear in
  `bashy agents` and `bashy agents --json` while live.
- A weave worker appears exactly once.
- Completion, error, and cancellation remove one-shot membership.
- External start/heartbeat/stop is PID-owned; heartbeat preserves original
  start time; lease expiry hides the assignment while `--all` retains it.
- A live raw shell-presence card does not increment `LIVE`.
- Interactive singleton behavior and `chat sessions` remain unchanged.
- Focused room/chat/agentos tests, full Bashy/Coreutils Go suites, and the
  Coreutils cross-platform gate pass.
- The installed Dragon `bashy` binary passes a live smoke using a harmless
  blocked fake/short agent invocation.
