---
id: 9f3cc23e7193
kind: bug
title: 'S407.4 P0 cert blocker: bashy sh leaks a pidfd + a /dev/null fd per external command on Linux (job-carrier path); VSC awk set times out'
seq: 1947
status: done
priority: p0
labels:
    - posix-cert
    - linux
created: 2026-10-25T01:59:18.058079Z
weave: 279
assignee: codex-gpt6-sol
sprint: 407
sprint_id: 90546935-fbc2-5d09-94b0-19c380846681
sprint_title: bashy 1.0.0 release gates pending external dependencies (follow-up to Sprint 379)
closed: 2026-10-25T02:46:59.430279Z
resolution: fixed
closed_by: codex-gpt6-sol-s
---

Found by the S407.3 Profile D replay of v1.0.0-rc.13-dev (bashy 4c37c968) on Ubuntu 24.04 x86_64, 2026-10-25.

Evidence (process table only, sampled from outside the test workspace):
- The VSC awk set (/tset/POSIX.cmd/awk/awk.ex, run by /vsc/sut/sh -> /vsc/stage/bashy as the sh personality) exceeded its 600 s cap twice; historical awk sets took 173-229 s under the same cap.
- /proc/<sh pid>/fd grows by one anon_inode:[pidfd] AND one /dev/null per executed command and they are never closed: 749 fds at 01:58:15 -> 781 at 01:58:45 (pidfd 368 -> 385, /dev/null 368 -> 384). Each external command also shows a child '/vsc/stage/bashy --bashy-job-carrier'.
- Unrelated but also wrong on a certification host: '/vsc/stage/bashy schedule daemon --interval 1s' was started under the test user (uid 5002) at 01:48:18 during the run. Find what spawns it and make sure no background daemon starts from a non-interactive sh/bash invocation (or under VSC_PROFILE=cert).

Do:
1. Reproduce on Linux (not macOS: pidfd is Linux-only; use a Linux container/VM/remote host, never podman on Dragon): a bashy sh script running ~2000 external commands, printing ls /proc/$$/fd | wc -l before/after. Red test first (fd count must be bounded; e.g. Go test asserting no fd growth across N external commands on linux).
2. Find the leak (likely the pidfd/job-carrier spawn path: pidfd from pidfd_open/clone and the /dev/null opened for the carrier's stdio are not closed after wait). Bisect against the last known-good candidate if unclear (Profile D 2026-10-06 ran bashy ba62cd91 with awk in time).
3. Fix at the root; also measure wall time of 2000 external commands before/after.
4. Find and stop the schedule-daemon autostart in non-interactive shells.
Acceptance: Linux regression test proving bounded fds; make test-bash 86/86 on a Linux host; awk-heavy loop timing comparable to ba62cd91. Commit with Sprint/Story/Story-ID trailers.
