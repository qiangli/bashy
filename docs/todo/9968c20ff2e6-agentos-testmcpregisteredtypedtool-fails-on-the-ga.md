---
id: 9968c20ff2e6
kind: test
title: 'agentos: TestMCPRegisteredTypedTool fails on the gate-host lane (cwd "" after EvalSymlinks)'
seq: 3319
status: todo
priority: p2
created: 2026-11-25T19:21:38.178529Z
---

Seen 2026-11-25 on the gate host lane 3 (gate-lane.sh env, GOWORK=off, GOTMPDIR under the lane), identically at bashy main 5d1df33e and the Sprint 721 g5 candidate 06dfd56c, so not a g5 regression. mcp_test.go:318: EvalSymlinks(interp.ShellPathToOS(dir, got.Dir)) returns "" (error ignored) while want is the t.TempDir() path under the lane tmp. Root cause NOT yet identified: next, print got.Dir and the EvalSymlinks error, check whether the registered fixture runs in a different working directory or mount namespace on that venue, and compare with the dev box. Classify (venue / test / product) before any fix.
