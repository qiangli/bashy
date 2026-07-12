---
id: 93d26791514d
kind: bug
title: 'weave status reports phantom ''killed: idle'' on a live run (idle > run lifetime)'
status: open
reporter: qiangli
created: 2026-07-12T23:22:22.716347Z
---

On host dragon-2, run #11 (codex, P1 chunks.json) shows in `weave list` as state=working with dur incrementing (7m11s→7m14s across two calls) and fresh PTY output, yet `bashy weave status 11` prints:

    state:    working
    killed:   idle 14m57s exceeds --idle-timeout 12m0s

The reported idle (14m57s) EXCEEDS the run's total lifetime (7m14s), which is impossible. The 'killed:' line is a phantom — no kill occurred, the run is demonstrably alive and progressing. Same false positive seen on #13 before it legitimately submitted.

Impact: an operator following the documented gate ('require killed: absent') would wrongly abandon a healthy, working run. The idle computation appears to use a stale or wrong baseline timestamp (possibly a shared/global last-activity clock rather than per-run). Likely same family as 8d598429 (submitted-with-0-commits) — the run-state accounting is unreliable.

Repro: launch a codex agent run, let it work >12m of wall time OR call status while another run idled, observe idle > dur.
