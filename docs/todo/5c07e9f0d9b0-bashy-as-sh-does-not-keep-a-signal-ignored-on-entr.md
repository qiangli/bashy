---
id: 5c07e9f0d9b0
kind: bug
title: 'bashy as sh does not keep a signal ignored on entry: SIGTERM ignored by the parent, ''kill -TERM $$'' still kills bashy (143)'
seq: 363
status: todo
priority: p0
labels:
    - posix
created: 2026-09-30T16:52:56.414753Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Found by the Sprint 340 conductor 2026-09-30 (coreutils weave #12 merge gate). Repro: python3 sets SIGTERM to SIG_IGN, then execs '<shell> -c "kill -TERM $$; printf survived"': /bin/sh -> rc 0 'survived'; ~/.local/bin/bashy.real -> rc 143, no output. POSIX.1-2017 XCU 2.11: signals ignored on entry to a non-interactive shell cannot be trapped or reset - the shell must keep them ignored. Impact: coreutils cmds/env TestEnvIgnoreSignal fails whenever sh on PATH is the bashy shim (~/.bashy/shims/sh -> bashy.real), which fails weave's full-suite merge gate for unrelated coreutils runs (worked around with PATH=/bin:/usr/bin:$PATH). Fix in the sh engine / bashy startup: record the inherited disposition and never reinstall a Go handler for signals that were SIG_IGN at entry. Red/green: the python repro above.

Sprint 341 Profile D diagnostic sh_03:164 and archived B both FAIL with seven inherited-ignored signals (ABRT, ALRM, PIPE, QUIT, TERM, USR1, USR2) terminating Bashy. C GNU Bash also FAILs the TP, but under one distinct default-SIGQUIT case. Do not treat the shared numeric code as one cause. Acceptance: demonstrate and repair SIG_IGN inheritance for Bashy/sh with a suite-free Linux process regression covering the seven signals; ensure the shell does not reset inherited ignored dispositions in its launch/exec path; run the exact licensed sh TP164 on the repaired pinned candidate and reconcile the B/D differential. Preserve C's separate GNU-control failure for authority review. D's running binary remains frozen.
