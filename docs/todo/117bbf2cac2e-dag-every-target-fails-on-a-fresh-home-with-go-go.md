---
id: 117bbf2cac2e
kind: bug
title: 'dag: every target fails on a fresh HOME with ''go: go.mod file not found'' before the body runs'
seq: 3317
status: todo
priority: p2
labels:
    - dag
    - fresh-home
created: 2026-11-25T16:03:27.443249Z
---

Seen 2026-11-25 (umbrella Sprint 712, testing the shapeware lifecycle dag.md). Repro on the dev box:
  mkdir -p /tmp/h && cd <folder with a trivial dag.md> && HOME=/tmp/h BASHY_HOME=/tmp/h/.bashy BASHY_HINTS=off bashy dag -f dag.md <target>
  -> go: go.mod file not found in current directory or any parent directory; see 'go help modules'
The target body never runs (an `echo` target prints nothing). With the normal HOME the same dag.md runs fine, and `bashy skill add .` works under the fresh HOME. So `bashy dag` itself invokes go during startup when HOME is fresh.
Same failure class as the 11-24 field defect "D1: go.bsh does not build on a fresh HOME" (umbrella Sprint 712 premise). Shapeware makes dag.md the lifecycle every agent runs (source/build/test/install/run), so a fresh install cannot run its own lifecycle until this is fixed.
Next: root cause first (what go invocation, from which dag startup path, and why only a fresh HOME), then a fix and a fresh-HOME regression test.
