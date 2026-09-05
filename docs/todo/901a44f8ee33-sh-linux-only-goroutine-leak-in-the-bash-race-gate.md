---
id: 901a44f8ee33
kind: task
title: 'sh: linux-only goroutine leak in the Bash++ race gate''s focused run'
seq: 218
status: todo
priority: p1
created: 2026-09-05T18:42:04.052785Z
sprint: 115
---

FOUND 2026-09-05 by making the Bash++ race gate platform-aware. Real defect, NOT introduced by that change.

EVIDENCE (ubuntu-latest, gate job "Bash++ race lifecycle gate"):
  --- FAIL: TestConcurrencyScheduleMatrix
      goroutine leak: 1 goroutine survived the focused run
      leak #1: goroutine [select, 2 minutes]
        created by interp.(*Runner).startSignalSubscriptionLocked
        in        interp.(*Runner).forwardSignalSubscription   interp/signal.go:730

WHY IT WAS NEVER SEEN: the gate aborted on linux at its exact-manifest check
before running a single test, so the LINUX HALF OF THIS GATE HAD NEVER EXECUTED.
Darwin runs the same gate and passes with no leak.

SCOPE HINT — the darwin and linux focused selections differ by exactly three
tests, so the leaker is very likely among them or is a shared test that leaks
only on linux:
  darwin only : TestDarwinNestedPipelineSIGPIPEIsolation
  linux  only : TestPipelineBuiltinSIGPIPEIsolation
                TestBackgroundExternalLaunchBoundary

ONE FIX ALREADY TRIED AND REVERTED (67c0bd82, reverted in ac6bb1fc): adding
`defer runner.Reset()` to runBackgroundLaunchScript, which builds a fresh Runner
per call in a loop without teardown. It did NOT stop the leak — same goroutine,
still parked — and it broke TestBackgroundExternalLaunchBoundary/"child is
launched and named by $! before the next statement". So that helper is not the
leaker, and Reset is not a safe drop-in there. Do not retry it blind.

WHAT THE NEXT ATTEMPT NEEDS: a linux reproduction. This cannot be diagnosed from
darwin — the gate passes there — and `go test -list` cannot cross-execute, so
GOOS=linux gives nothing locally. Run the focused selection in a linux container
and bisect the selection until the leak disappears; the goroutine report already
names the creation site, so the remaining unknown is only WHICH test leaves a
runner alive.

DO NOT: add the test to a known-failures baseline, widen the leak allowlist, or
drop the two linux rows from the manifest. The rows are required — the gate
checks selected-subset-of-manifest in both directions — and the leak is a real
signal-subscription lifecycle bug, which is squarely this sprint (runtime
safety).
