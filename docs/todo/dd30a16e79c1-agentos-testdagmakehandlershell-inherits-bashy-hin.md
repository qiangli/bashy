---
id: dd30a16e79c1
kind: test
title: 'agentos: TestDagMakeHandlerShell inherits BASHY_HINTS=off from the venue and fails ''hint count'''
seq: 3319
status: todo
priority: p3
created: 2026-11-25T18:26:28.798552Z
---

Seen 2026-11-25 on the gate host lane (gate-lane.sh env exports BASHY_HINTS=off), identically at bashy 4bf2bbe0 (main) and the Sprint 721 g2 candidate f0605535. The interp run in dagmake_test.go:270 uses the process environment, and dagmake.go:357 silences the hint when BASHY_HINTS=off. Classification: test defect (venue-dependent). Fix: set BASHY_HINTS explicitly (unset/on) in the test's interp env. Not a regression.
