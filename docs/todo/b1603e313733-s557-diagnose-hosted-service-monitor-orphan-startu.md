---
id: b1603e313733
kind: bug
title: S557 diagnose hosted service-monitor orphan startup port ownership failure
seq: 3042
status: assigned
priority: p1
created: 2026-11-01T23:01:13.521937Z
weave: 3
assignee: claude-opus5.5
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

## Review rework (613f2a1 → this commit)

Independent review (`/tmp/s557-service3042-review-613f2a1.md`) required three
corrections; all are made.

1. **Read errors are not completion.** Liveness is now tri-state: live, proven
   gone (an error wrapping `errProcessGone`), or unknown (any other error).
   Only a vanished `/proc` entry (ENOENT/ESRCH) or a counted-out thread group
   proves exit; EACCES, EIO and malformed stat stay unknown. `sameProcess`
   (the signal condition) still requires proven identity, but it is no longer
   the completion test. `processMatches` returns the error, and its callers
   fail closed: reclaim keeps the ownership record and reports `liveness
   unknown` for an unreadable orphan or monitor; `waitProcessesGone` waits for
   a member until it is *proven* gone (`provenGone`); group capture fails
   instead of dropping an unreadable member. The boot-id read error, which
   was silently ignored before, is now unknown, so it can no longer produce a
   mismatched identity. Darwin now uses `SysctlKinfoProcSlice`, because
   `SysctlKinfoProc` returns EIO for an absent pid (x/sys source) and would
   now read as unknown. Windows maps ERROR_INVALID_PARAMETER and a
   non-STILL_ACTIVE exit code to proven gone.
2. **No completion from a missing/empty task directory.** The proof is
   `num_threads` (field 20) from the leader's own stat snapshot. That is the
   same snapshot that names the incarnation, so it cannot mix two
   incarnations. Per the kernel source the review cites, a non-leader thread
   is released (and the count decremented) only after its `exit_files`. A
   zombie leader counting at most itself therefore owns no descriptors.
   `>1` is live. An unparsable or negative count is unknown. `/proc/PID/task`
   only corroborates: it can keep a group live but never prove it gone. A
   missing or empty listing decides nothing beyond the count. Task-stat EACCES,
   EIO or malformed data is unknown, and a vanished task is skipped. A running
   task the count did not include triggers a re-read of the leader. The group
   is live only while that re-read shows the same start time: a changed or
   vanished leader is proven gone, and a failed re-read is unknown. The 0-count
   case (the leader itself mid-release) is treated as gone. Neither that case
   nor the count's ordering has been observed on a hosted kernel; both rest on
   source review.
3. **Listener retained.** The leader-thread-exit child stores its listener in
   the package variable `leaderExitListener`.

The /proc decision logic moved to the untagged `supervisord_procstat.go`
(`procFS`, with injectable reads). Linux wires it to the real `/proc`, and the
fault tests run on every platform. New tests:
`TestProcStartTimeProvesExitOnlyFromVanishingOrThreadCount`,
`TestProcStartTimeRevalidatesLeaderIncarnation`,
`TestProcGroupIdentitiesFailsOnUnknownMember`,
`TestReclaimKeepsOwnershipWhenLivenessUnknown`. A mutation that restores
"ignore task-stat and task-dir errors" fails four of the subtests.

Unchanged: signaling targets, group selection, identity matching, grace, the
5 s final join, the 10 s monitor deadline, and the foreign-listener and
reused-pid guards. No sleeps were added, and nothing is identified by port.

Dev evidence (darwin/arm64, `-race -count=3`, 4.904s): the four new tests,
TestReclaimOrphans*, TestCapturedOrphanEscalationSparesChangedIdentity,
TestServiceMonitorReclaimsOrphanOfHardKilledMonitor and
TestProcessProviderOwnershipRecordLifecycle all passed. `go vet` was clean for
darwin, linux/amd64, linux/arm64 and windows/amd64.

**Not run on Linux.** No hosted Linux pass is claimed. The original hosted
port-held failures remain unresolved. Hosted selector to run:

    GOWORK=off go test -race ./internal/agentos -run '^(TestReclaimOrphansWaitsForThreadsOfExitedLeader|TestReclaimOrphansJoinsTermIgnoringGroupListener|TestReclaimOrphansSparesUnprovenProcesses|TestCapturedOrphanEscalationSparesChangedIdentity|TestServiceMonitorReclaimsOrphanOfHardKilledMonitor|TestProcStartTimeProvesExitOnlyFromVanishingOrThreadCount|TestProcStartTimeRevalidatesLeaderIncarnation|TestProcGroupIdentitiesFailsOnUnknownMember|TestReclaimKeepsOwnershipWhenLivenessUnknown)$' -count=3 -timeout=120s

On failure, record `uname -r` along with the `portDiagnosis` output, which
covers dial/bind, /proc/net/tcp states and task states.


Independent review correction — Codex repair_review internal helper, root delegated:

Reviewed Claude commits613f2a1 and72ea8d04 without rewriting either. The tri-state completion callers, count-based conservative Linux task handling, Darwin absent-process distinction, Windows gone-error mapping, and explicit global test-listener lifetime address the three prior review concerns. Linux execution remains pending.

One remaining full-length malformed-stat case was identified: a nonnumeric field22 could be compared as a different incarnation, while a nonnumeric field5 could silently exclude a group member. Added only numeric validation of consumed pgrp/starttime fields before either identity or membership decisions. Errors remain unknown rather than proven gone. Six authored cases cover text, negative and overflowing values for both fields; group capture must also reject them. No signal targets, grace/deadlines, ownership assertions or platform selection changed.

Focused Darwin race selection (unknown liveness, task/read errors, leader revalidation, malformed identities, TERM-ignoring listener, foreign/reused identity protections and hard-killed monitor) PASS3.571s. Prior rework without numeric addition passed the same selection minus new cases in3.643s. Negative-control overlay restoring72ea8d04 procstat source fails all6 new cases (1.477s total), proving the guard is not vacuous. Canonical filesystem overlay paths used. No full suite/build or Linux fixture execution. Hosted Linux must include TestReclaimOrphansWaitsForThreadsOfExitedLeader and these new fault cases; neither original hosted cause nor full lifecycle acceptance is claimed by Darwin success.
