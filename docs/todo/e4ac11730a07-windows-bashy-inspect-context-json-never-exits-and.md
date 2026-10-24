---
id: e4ac11730a07
kind: bug
title: 'Windows: ''bashy inspect context --json'' never exits and burns a full core (busy loop) when launched non-interactively (cmd /c over ssh)'
seq: 1945
status: done
priority: p1
labels:
    - windows
created: 2026-10-24T00:29:58.698973Z
weave: 274
assignee: agy-gemini3.8-flash
sprint: 497
sprint_id: 16af0380-7ec7-5d41-a9c6-50e1b03cdf11
sprint_title: 'Fix the 2026-10-23 findings: satellite login/superadmin security, web console multi-satellite UX, fleet downgrade trap, Windows agent bugs, weave/sprint tooling, outpost headless/version skew'
closed: 2026-10-24T06:35:45.084747Z
closed_by: s497-manager
---

Found 2026-10-23: on the approved Windows test host, two 'cmd /c bashy inspect context --json' processes from 11:14 (agent ssh sessions) were still running at 17:28, about 22,600 CPU-seconds each (2 cores together, CPU load 45%, fan noise). Killed. Reproduced with the newly installed build (bashy 49a3a6f0): Start-Process bashy.exe inspect context --json with redirected stdout was still running after 20 s with 20.6 CPU-seconds used. inspect context is the documented first call for agents, so every agent session on Windows can leave a spinning process. Expected: it completes in well under a second and exits; add a regression test that runs it with redirected stdio on Windows.

## Worker brief (s497-manager, 2026-10-23). One repo: bashy.

Likely root cause (verify): internal/agentos/context.go:612 `detectProjectRoot` loops `for dir := cwd; dir != "" && dir != string(filepath.Separator); dir = filepath.Dir(dir)`. On Windows filepath.Dir("C:\\") == "C:\\" which never equals "\\", so with no marker (.git, go.mod, README.md, .benchmark) up to the drive root it spins forever. Typical for ssh `cmd /c` sessions starting in the home dir. Reached from collectContext (:235) and coord.go:200. Also grep the repo for the same loop shape (`!= string(filepath.Separator)`) and fix every instance.
Fix: stop when `parent := filepath.Dir(dir); parent == dir` (works on all OSes, incl. UNC/volume roots).
Tests: internal/agentos/context_test.go: unit test that calls detectProjectRoot on a markerless temp dir and must return within a deadline (run the walk in a goroutine with a 5s timeout); plus a pure-logic test of the loop helper with Windows-style roots (factor the walk into a helper taking a dirFn so "C:\\" can be simulated on any OS).
Gate: `go test ./internal/agentos/ -run 'Context|ProjectRoot'` and `go build ./...`, plus `GOOS=windows go build ./...`. Do not run make test-bash. Trailers for this story only. No hostnames.
