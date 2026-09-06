---
id: 36494de0fef8
kind: task
title: 'P0: Tool-managed inbox watcher must establish the authored session claim'
seq: 227
status: done
priority: p0
created: 2026-09-06T01:03:06.795888Z
sprint: 127
closed: 2026-09-06T04:02:34.502106Z
---

MEASURED FAILURE: holding a sprint lease alone did not let an external manager author delegation notifications. SECURITY DECISION: the lease must NOT become identity; it is public accountability state, and accepting it with self-set BASHY_AGENT_ID lets a bystander impersonate the live holder. The rejected coreutils weave run #8 demonstrated that trap and must not merge. ROOT CAUSE: the Tool-managed workflow established/streamed the sprint lease but did not explicitly prove the manager session through its inbox watcher before authoring. EXISTING SECURE PATH IS GREEN: in a hermetic HOME, a registered manager ran `bashy inbox --as probe-manager --watch` under the actual external tool session; `bashy notify --as probe-manager probe-worker` returned rc=0 and the worker unified inbox contained the exact body. SPRINT 127 FIX SCOPE: tests and guidance only. The Tool-managed E2E must start the independently-running inbox watcher/session claim, then prove assignment/notify delivery through bus.UnreadNotifications or the unified inbox. Update conductor guidance to make that ordering explicit. Do not weaken bus.ResolveAuthoredActor, add lease-as-identity, or invent another identity system.
