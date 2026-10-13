---
id: e863c096ad55
kind: bug
title: 'Repair hosted Prima no-egress gate and CI evidence'
seq: 1907
status: done
priority: p1
labels:
    - prima
    - builds
    - no-egress
created: 2026-10-13T07:00:00Z
sprint: 436
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
---

Candidate `bashy-bashy-prima-0.1.0-2` (hosted run 38411861522) compiled all
four platforms but the macOS no-egress gates failed with `sandbox observer
cannot see control denial`.

Root cause: the Darwin branch proved sandbox enforcement through the unified
log (`log show` grep for the control denial), which is not a reliable
observer for sandbox denies — no denial entries are visible via that
predicate even on a local darwin-arm64 host where the control connection is
demonstrably denied.

Fix (worker `c326ac82`, merged as `03eeae61`): the gate now observes the
denial directly in the attempt's own stderr (`PermissionError: [Errno 1]
Operation not permitted`). Control success fails closed, control failure
without an EPERM signature fails closed, and only a directly-observed EPERM
passes the positive control. The engine phase fails on non-zero exit under
the deny profile and on any EPERM signature in the engine log; the
unified-log sweep is retained as a fail-closed signal only and its absence
neither passes nor fails the gate. Sandbox profile unchanged (deny
network-outbound, six explicit loopback ring ports). Linux strace verdict
path untouched. No signing claim; no policy weakening.

Verified: `builds/check.sh` prima.cpp, `check_test.py` (11 tests, 4 new),
shellcheck, actionlint, live darwin-arm64 probe of all four observer
behaviors. Hosted logs could not be pulled from the worker (no GitHub auth).

Follow-up: the Linux legs of the same run failed with no cause in the hosted
logs at all — see story `ad6b70841ac3`.
