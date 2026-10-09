---
id: e27d4c067f49
kind: bug
title: 'bashy git diff --stat and show --stat/--format fail: built-in cobra verbs shadow the engine'
seq: 420
status: todo
priority: p1
labels:
    - git
created: 2026-10-09T05:25:20.298812Z
---

`bashy git diff --stat`, `bashy git show --stat HEAD` and `bashy git show -s --format=%h` die with "unknown flag" because the dedicated cobra diff/show subcommands shadow the engine's nativeDiff/nativeShow. Same shape as the log fix in Sprint 404: route flags beyond the cobra verb's own set to outgit.Exec.  Found by the Sprint 404 smoke (story 2324bae4d6fa) of bashy git through the native one-door; fails identically with and without -C, so it is independent of the -C fix.  (found by the Sprint 404 smoke, 2026-10-09; native engine only, works with --external)
