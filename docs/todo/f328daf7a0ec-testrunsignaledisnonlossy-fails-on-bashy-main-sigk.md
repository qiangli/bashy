---
id: f328daf7a0ec
kind: bug
title: TestRunSignaledIsNonLossy fails on bashy main (SIGKILL not reported as signaled)
seq: 1938
status: todo
priority: p2
created: 2026-10-18T08:10:11.774672Z
---

Fails on bashy main 58abd06d and 0d2af6873 (not caused by Sprint 457). Repro: GOWORK=off go test ./internal/agentos/ -run TestRunSignaledIsNonLossy. Output: run_test.go:51: a SIGKILL'd process should report signaled=true. FAIL github.com/qiangli/bashy/internal/agentos.
