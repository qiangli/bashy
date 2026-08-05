# Plan: event-driven `bashy await`

## Problem

Long-running agent work and remote test campaigns currently have uneven
supervision. `bashy weave wait` wakes on a story's terminal state and ycode can
emit factual `turn.start`, `tool.call`, and `turn.end` events, but files,
processes, remote systemd units, journals, and webhooks lack one conductor-facing
contract. The result is repeated status polling and progress that is only
noticed after a human asks.

The shell detecting an event is only half of the problem. Bashy alone cannot
inject an event into a model conversation. A live session host can expose a
wake/steer operation—even while the model is idle—but no in-session mechanism
can resurrect a process or session that has terminated. The durable owner must
therefore be ycode, the conductor, or another app-server process that remains
subscribed and can wake, steer, queue for, or resume the agent.

## Proposed surface

```sh
bashy await agent agy
bashy await file /vsc/logs/run.ledger --contains ARM_DONE
bashy await process 1234
bashy await systemd vsc-bashy-cu-capped --host do-droplet
bashy await webhook --event test.completed
```

Every source emits a versioned NDJSON envelope with at least `source`, `kind`,
`state`, `observed_at`, `terminal`, and source-specific evidence. Human-readable
output may be a view of that envelope, never a separate source of truth.

## Boundaries

- Reuse `weave wait`; do not build a second agent-run monitor.
- Consume ycode's declared event stream; do not infer turn completion from
  silence.
- Use native filesystem facilities through the existing permissively licensed
  watcher foundation: FSEvents, inotify, and ReadDirectoryChangesW.
- For remote units, keep one bounded noninteractive SSH connection and consume
  systemd/journal events. Reconnection must be explicit in the evidence.
- A timeout is a safety boundary, not evidence that the watched work failed or
  hung.
- Polling is permitted only when a source has no subscription interface. The
  envelope must declare that fallback and its interval.
- Delivery is at-least-once. Include an event identity so conductors can
  deduplicate after reconnect or restart.
- Terminal events must never be coalesced or dropped. Progress events may be
  coalesced under backpressure.
- Watching does not grant authority to mutate, restart, kill, merge, or deploy.

## Ownership

- **Bashy/coreutils:** event-source adapters, normalization, lifecycle,
  reconnection, NDJSON schema, and local CLI.
- **ycode/conductor:** durable subscription registry, deduplication cursors,
  thread association, replay/checkpoint, wake/steer/resume policy, progress
  summaries, webhook authentication/routing, and escalation to the human.
- **Provider adapters:** translate Codex/Claude/other native events without
  weakening the common evidence contract.

Do not make Bashy a webhook daemon or control plane. It may expose a foreground
stdin/socket adapter, but the durable listener belongs to the conductor. Consume
ycode's native `--events` stream directly rather than converting it into files
for a watcher to rediscover.

## Claude Code comparison

Claude Code provides lifecycle hooks including `SubagentStop`, `TaskCompleted`,
`Notification`, and `FileChanged`, plus background task identities and status
views. Its asynchronous hook `asyncRewake` demonstrates the desired lifecycle:
a hook can wake a live, idle Claude session and inject a system reminder.
Ordinary asynchronous output waits for a later turn, and a terminated Claude
process cannot be revived. Bashy's adapters should supply portable events;
ycode's session host should provide the analogous controlled re-wake operation.

References:

- <https://code.claude.com/docs/en/hooks>
- <https://code.claude.com/docs/en/interactive-mode>
- <https://code.claude.com/docs/en/agents>
- <https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode>

## Acceptance criteria

1. A hermetic file change wakes one waiter without an interval loop and reports
   the changed path and native source.
2. A completed `weave` story produces exactly one deduplicated terminal event
   through the shared contract.
3. A ycode `turn.end` is forwarded byte-for-byte as a declared agent event; a
   quiet interval never becomes a synthetic completion or hang.
4. A remote systemd fixture streams progress, survives one forced SSH
   reconnect, resumes from a journal cursor, detects reboot or unit replacement,
   and reports the final unit state with journal evidence.
5. A conductor integration test proves an event can steer or resume the owning
   live thread after the original command turn; a dead session queues the event
   for explicit resume rather than claiming it was delivered.
6. Cancellation removes watchers and remote processes with no orphaned child.
7. Unsupported native subscriptions fail closed or identify bounded polling in
   their output; they never silently poll.
8. File tests cover create, change, delete, rename, atomic-save replacement, and
   watcher overflow/rescan. Conductor tests cover restart and redelivery of an
   unacknowledged terminal event.
9. Webhook tests cover authentication, idempotency, and replay protection; event
   payloads and logs pass the normal secret-redaction boundary.

## Open design questions

- Where subscription state and deduplication cursors live across restarts.
- Which Codex App Server and Claude hook/background-task events are stable
  enough for first-party adapters.
- Whether remote journal transport belongs in Bashy or the outpost mesh, with
  Bashy consuming an outpost event stream.
