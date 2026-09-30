---
id: 401837b6352f
kind: task
title: Refresh POSIX preflight and yash evidence
seq: 36
status: todo
priority: p1
created: 2026-09-03T00:23:52.704849Z
sprint: 110
---

Run posix-certdryrun, the yash POSIX scoreboard, and the public differential matrix against the same frozen candidate. Require non-empty verdict sets and record any runtime-sensitive signal, file-descriptor, process, or diagnostic delta.

## Review 2026-09-30 (steward)

- Status: valid, not run for this candidate. Runners exist today: `scripts/posix-certdryrun.sh`, `scripts/yash-scoreboard.sh` (+ `yash-posix-suite.sh`), `scripts/posix-diff.sh` / `posix-parity-allshells.sh`, `scripts/dash-posix-suite.sh`.
- Outdated: no candidate or prior numbers were named. The prior reference is the last recorded dry-run/yash result in `docs/conformance-statement.md` / the runbook history; name it explicitly before running.
- Next step: run the four runners against the frozen candidate on the Linux runner (containerized, not a dev host); record VERDICT line, per-suite verdict counts and the yash score next to the previous values.
- Acceptance: non-empty verdict sets for every suite; VERDICT CLEAN or each deviation bucketed (real bug / declared limitation / scope-excluded); yash score not below the prior record; signal, fd, process and diagnostic deltas listed.
- Depends on: ae9b5ef7e7e8, 1727b5446a3b. Parallel with 34046901d606.
- Addendum (2026-09-30 guidance): dry-run, yash and differential suites are substitute harnesses - preflight only, never certification evidence. Record them as such.
