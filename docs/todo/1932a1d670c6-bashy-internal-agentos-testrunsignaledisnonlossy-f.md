---
id: 1932a1d670c6
kind: bug
title: 'bashy internal/agentos: TestRunSignaledIsNonLossy fails on main (a SIGKILL''d process should report signaled=true)'
seq: 2824
status: todo
priority: p2
labels:
    - test
    - darwin
created: 2026-10-29T02:14:30.220259Z
---

Found at Sprint 522 close-out 2026-10-28 on Dragon (darwin/arm64): GOWORK=off go test ./internal/agentos/ fails only TestRunSignaledIsNonLossy (run_test.go:51 'a SIGKILL'd process should report signaled=true'), deterministic (-count=3), identical on unmodified origin/main before the S522 yoke pin bump. Everything else in the package passes.
