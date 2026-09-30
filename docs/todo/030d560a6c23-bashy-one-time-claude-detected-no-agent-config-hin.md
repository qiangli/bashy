---
id: 030d560a6c23
kind: bug
title: bashy one-time 'claude detected, no agent config' hint leaks into a child sh's stderr and breaks sh interp TestJobCarrierExternalChildPublishesPID
seq: 358
status: todo
priority: p2
labels:
    - hints
created: 2026-09-30T11:47:41.17471Z
---

Found 2026-09-30 (Sprint #110) on dragon where sh on PATH is bashy: the test runs sh -c ... and compares captured output; a fresh t.TempDir cwd makes the per-repo hint fire every run, so the test fails on the base sh too. Passes with BASHY_HINTS=off. A non-interactive child shell should not print onboarding hints (or only to a TTY).
