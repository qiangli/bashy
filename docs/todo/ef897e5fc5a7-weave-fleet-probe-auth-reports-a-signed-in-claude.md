---
id: ef897e5fc5a7
kind: bug
title: 'weave fleet --probe/--auth reports a signed-in Claude binding as ''waiting for a human: sign-in'''
seq: 3322
status: todo
priority: p1
labels:
    - fleet
created: 2026-11-26T01:16:00.264136Z
---

Observed 2026-11-25 (Sprint 722 Phase B), dev box (macOS).
- The operator signed in through `bashy claude` (managed claude 2.1.325). Afterwards, `<cache>/bin/claude/2.1.325/claude -p ... --model claude-sonnet-5` answers headless under the real HOME.
- `bashy weave fleet --fleet claude-sonnet5 --probe` still printed the cached 16:35 verdict 'UNUSABLE waiting for a human: sign-in', and `--auth` printed 'NEEDS-LOGIN'. So `sprint assign` would refuse a working worker.
- Related evidence: macOS Claude, agy (>=1.5) and muse keep their logins in the login Keychain, resolved through HOME and the config dir. In an isolated HOME or CLAUDE_CONFIG_DIR they read as signed out. agent-bench's arm hit exactly this, and its opt-ins are BENCH_CLAUDE_KEYCHAIN, BENCH_AGY_KEYCHAIN and BENCH_MUSE_KEYCHAIN (agent-bench 536c017).
Cause: pending. Hypothesis to test first: the probe runs the vendor CLI with a redirected HOME or config dir, so Keychain logins are invisible to it. Also check whether --probe re-probes at all, or serves the cached row (the timestamp did not change).
Classification: suspected product defect (a false negative in the readiness gate).
Fix direction, after the root cause: the probe must evaluate the same credential resolution the real launch uses, and --auth must never return a cached verdict. Add a regression test with a Keychain-backed fake.
