---
id: b1603e313733
kind: bug
title: S557 diagnose hosted service-monitor orphan startup port ownership failure
seq: 3042
status: assigned
priority: p1
created: 2026-11-01T23:01:13.521937Z
assignee: s557-manager
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
---

Hosted fullBashy41356815864 at4b3e283: TestServiceMonitorReclaimsOrphanOfHardKilledMonitor fails10.17s listenerneveractive foreignport41325. Preserve ownership/orphan assertions,10sdeadline, realprocesschecks; diagnose race or fixturecollision beforechange. Evidence private local /tmp/s557-bashy-full-41356815864-failed.log; public run URL supports sanitized evidence. Bounded focused authoredtests only ondev; fullsuitehosted. No release. Rootdelivery trailersrequired.


## Implementation evidence (internal helper, s557-manager)

The original hard-killed-monitor regression passed locally three times. Its
hosted startup failure remains unproven; this repair does not classify that
failure as resolved without hosted evidence.

A deterministic additional regression starts a process-group leader whose
same-group child ignores TERM and owns a ready-signaled TCP listener. Before
the repair, reclaim returned after the leader exited while that child remained
alive (focused test failed in 1.653s). The repair snapshots live member PID/start
identities on Linux/macOS, rechecks the leader before initial group signaling,
waits for captured members, and escalates only individually rechecked identities.
It never uses an occupied port to identify a process or signals a possibly
recycled group ID during escalation. Grace and the existing five-second final
join bound are unchanged. Snapshot errors preserve the ownership record.

The guarantee covers captured group members, not arbitrary newly spawned or
detached descendants. Other platforms preserve direct-child semantics. Existing
live-monitor, reused-PID, foreign-listener checks and original deadlines remain.

Verification: new group regression and original monitor regression passed three
runs (3.230s). The Linux snapshot helper compiled in an isolated temporary
stdlib-only module for linux/amd64. No product build, full suite, hosted dispatch,
or release action was performed.

Final focused race check passed (5.139s): TestReclaimOrphans*, original
hard-killed-monitor regression, captured stale-identity escalation negative,
and ownership-record lifecycle. The existing live-monitor/reused-PID and
foreign-listener assertions were included. `git diff --check` passed.

Independent review tightened escalation to acquire the process handle before
rechecking the captured identity and release all handles. Darwin retains its
inherent non-atomic PID/start-check-to-signal race; this change does not claim
kernel-level PID-reuse immunity there.

After the handle-order correction, the same focused race selection passed
again (3.492s). Independent read-only review found no further required changes.
