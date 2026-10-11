---
id: 675eceb862b9
kind: feature
title: 'S16 bashy supervisord becomes the service monitor: reconcile units, health probes, restart policy, start limits, live reload, status socket, PID1 mode'
seq: 1885
status: done
priority: p1
labels:
    - yoke
created: 2026-10-11T05:07:45.651934Z
weave: 23
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
closed: 2026-10-11T08:17:01.373168Z
resolution: fixed
closed_by: claude-opus5.5-w
---

Design docs/bashy-service-design.md: inetd/systemd split - bashy service owns unit definitions and per-unit verbs; bashy supervisord (a bashy command, like inetd) is the service monitor; outpost supervisord (init, like systemd) starts and watches only the outpost daemon and bashy supervisord. Repurpose bashy supervisord (bashy/internal/agentos/supervisord*.go, today one dag root in the foreground) as the long-running monitor over every enabled unit in bashy service list: simple (foreground) process model, monitor owns each process group; health http|tcp|exec with interval/timeout/failures (replaces grepping status text for stopped); restart policy always|on-failure|no with min/max backoff and healthy_after; start_limit burst/interval moves a unit to failed (no infinite crash loop); requires ordering; live reload on unit add/set/enable/disable with no restart of other units; rotating logs under BASHY_HOME/service/NAME; structured status on a local socket used by bashy service status/start/stop; service.* events on the room timeline (S3) and failed to notify sinks (S7); --pid1 reaps orphans and forwards TERM/INT (container entrypoint, no outpost needed). Reuse the existing supervisord backoff/signal/reap code. Acceptance: unit tests for backoff, start-limit failed state, health-failure restart, foreign listener on a unit port reported as failed with reason, live enable/disable, PID1 reaping on Linux; status --json schema golden file.

Operator rod-not-fish requirement 2026-10-10 PDT: implement reusable service declaration/configuration/enablement machinery shared by built-in, internal and external services. Built-ins are seeds, not service-name branches. Internal capabilities retain their subsystem behind a generic runtime provider and use the same registry, desired state, health/status/events and exposure contracts; no separate internal feature-flag path or fake child process. Acceptance includes seeded built-in + internal capability + fresh custom external declaration using the same service verbs, plus an operator override, without per-service supervisor/toggle/exposure code. Agree the S15/S16/S18 provider contract first; preserve complete existing feature coverage and no legacy compatibility/migration.

Conductor 2026-10-11: S15 provider interface is merged in yoke main. Own `bashy/internal/agentos/supervisord*` and monitor/runtime code; coordinate the shared IPC/status contract with S18 and keep S19's DAG caller switch separate. Verify on a claimed Linux venue with `go test ./internal/agentos/...` from bashy and `go test ./pkg/service/... ./pkg/fleet/...` from yoke, plus explicit PID1/reaping and live unit toggle probes. Commit with `Agent:` trailer.
