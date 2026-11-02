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


## Linux follow-up assignment

Exact candidate58a2442 native run41370643706 fails new TestReclaimOrphansJoinsTermIgnoringGroupListener on BOTH Linux jobs125040178282 and125040178542 at line146, after only0.19s: captured child identity considered gone but listener still held. Darwin focused success does not establish Linux correctness. Diagnose Linux completion semantics and distinguish active connection versus bind failure/TIME_WAIT before repair. Linux /proc pgrp/start indices were independently checked correct; helper announces ready only after IgnoreTERM and Listen. Hypothesis of zombie leader while other Go threads release sockets is unproven. Preserve identity-safe signaling, foreign-listener/PID-reuse guards, all existing deadlines and original monitor test. Do not add sleeps, relax ports/deadlines, or kill by port.

Assigned implementation scope: source review and bounded authored diagnostic/regression fixes in isolated managed workspace. No local full builds/suites/containers on Dragon. Native Linux validation requires manager-coordinated hosted lane or dedicated test host; do not touch the private POSIX runner. Commit with Sprint557/Story3042 trailers, report exact evidence and any unproven hypothesis. Root owns independent review, merge, hosted dispatch and consumer pins.

## Linux follow-up diagnosis and repair

Evidence. Both failing jobs report `port N/tcp (127.0.0.1:N) is held by a
foreign listener` at line 146. `checkPorts` formats its dial-accepted path and
its bind-EADDRINUSE path identically, so the hosted log alone cannot separate an
active listener from a bind failure. On Linux, however, TIME_WAIT cannot cause
the bind failure: Go sets SO_REUSEADDR on every listener, and the only sockets
on the port are the listener and the backlog child of the precondition probe
(aborted with RST when the listener closes). Either path therefore means a LISTEN
socket still exists on the port. A different process binding the
just-released port inside 0.19s on both jobs is the less likely explanation. It
is not excluded by the hosted evidence.

Mechanism (kernel semantics, not yet observed on a hosted Linux runner).
`/proc/PID/stat` reports only the thread-group leader's state. Under SIGKILL every
thread runs its own exit path. The leader can reach exit_notify and show `Z`
(a delayed group leader) while sibling threads have not yet run exit_files on the
shared descriptor table, which still holds the listening socket. Go processes always
have several threads, and a loaded runner can delay the scheduling of the sibling
threads. `procStartTime` and `processGroupIdentities` treated a `Z` leader as
gone, so `waitProcessesGone` could return before the socket closed. The same
mechanism also explains the original hard-killed-monitor failure: reclaim returns
early, and the successor's `checkPorts` sees the still-closing orphan as a
foreign listener, a failed state with no retry.

Repair. On Linux, a process is live while any task in `/proc/PID/task` is short
of zombie (`threadGroupLive`). A non-leader task leaves that directory only
after it has released its files. An unreadable but present task directory counts
as live. This applies to identity (`procStartTime`) and group capture. The PID
cannot be recycled while the zombie leader exists, so identity-safe signaling
is unchanged. Deadlines, grace, the guards against foreign listeners and reused
PIDs, and the original tests are unchanged. No sleeps were added, and nothing is
identified by its port.

Regression. `TestReclaimOrphansWaitsForThreadsOfExitedLeader` (Linux only) makes
the mechanism deterministic. A child exits only its main thread (raw SYS_EXIT
from init, which Go runs on the main thread) while its runtime threads keep the
listener. The test asserts that identity, group capture and reclaim completion
all treat that process as live. Failures in this test and at line 146 now
append `portDiagnosis`: separate dial/bind results, `/proc/net/tcp{,6}` states for
the port, and the task states of each PID.

Validation status: compiled and vetted for linux/amd64, linux/arm64 and windows.
The focused darwin race selection (TestReclaimOrphans*, the hard-killed-monitor regression, captured stale-identity escalation, ownership-record lifecycle) passed on dev in 10.572s. The
new regression and the repair have NOT run on Linux: that needs the
manager-coordinated hosted lane.
