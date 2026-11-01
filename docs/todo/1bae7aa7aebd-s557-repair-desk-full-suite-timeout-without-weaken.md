---
id: 1bae7aa7aebd
kind: bug
title: S557 repair Desk full-suite timeout without weakening behavioral coverage
seq: 3041
status: assigned
priority: p1
created: 2026-11-01T19:27:07.086888Z
weave: 2
assignee: s557-manager
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
---

Sprint557 migration repair. Work only in assigned isolated workspace. Read local CLAUDE/AGENTS. Preserve all test assertions, coverage, thresholds and deadlines; diagnose before modifying. No release/push/parent pin changes. No heavy/full local builds/suites: bounded targeted tests only GOMAXPROCS=2 GOFLAGS=-p=2, full validation hosted by manager. Commit named files with Sprint: #557 and correct Story/Story-ID trailers and injected BASHY_AGENT. Report exact evidence and limitations. Full hosted run41332672873 atBashy86021a96 failed only examples/desk timeout10m; TestDeskToolsForkAndRollback chart/form waiting child CombinedOutput. Log /tmp/s557-bashy-full-41332672873-failed.log. Inspect nested go-run build/runtime phases; eliminate redundant build work or actual leak without increasing behavior deadlines. Keep all tool fork/rollback scenarios.

## Hosted timeout diagnosis and instrumentation

Run 41356815864 still hit the existing 600-second package deadline after the installer/fork integration. At its source revision, `shaped_fork_test.go:64` is `src undo`, not second activation. The sole reported running subtest was brainstorm (2m37s total subtest age). Its stack was waiting for the child process itself; the log does not establish how long undo had run or whether verification, cloning, or earlier package stages consumed the budget. Both fork/verify/activate rounds and their active-revision assertions precede this command.

Undo uses Yoke `UndoSealed` → `RestoreSealed` → `verify.Run`: fresh verification is intentionally bound to the current history tip before the rollback gate. Do not remove it as redundant work without changing the contract. There is presently no evidence supporting another deduplication or deadline change.

The next diagnostic checkpoint preserves every command, environment, assertion, and existing deadline. A shared test-only command helper emits start/end elapsed time, package elapsed time, child PID, and a bounded 4-KiB captured-output tail every 30 seconds while pending. Complete combined output still reaches existing assertions. Traces cover the product build, all fork commands, standalone server builds, Go cache discovery, and health probes. Direct package diagnostic output retains pending-stage evidence even when the package alarm interrupts a subtest. No production behavior changes.

Validation: `GOWORK=off GOMAXPROCS=2 GOFLAGS=-p=2 go test -race examples/desk/command_trace_test.go examples/desk/command_trace_helper_test.go -run '^TestDesk(CommandTrace|TraceChild)$' -count=1 -timeout=30s` passed in 2.346s. Authored child-process tests verify complete stdout/stderr preservation, failure exit status, pending-stage reporting, and diagnostic-tail truncation under the race detector. No product build or full Desk scenario ran locally. Hosted follow-up should run the complete Desk package with `-v -count=1 -timeout=600s`; all six tools, both forks, verification, activation, and rollback remain mandatory. This checkpoint diagnoses the timeout and does not claim it fixed.
