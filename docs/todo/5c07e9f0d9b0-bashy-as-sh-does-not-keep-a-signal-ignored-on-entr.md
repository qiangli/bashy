---
id: 5c07e9f0d9b0
kind: bug
title: 'bashy as sh does not keep a signal ignored on entry: SIGTERM ignored by the parent, ''kill -TERM $$'' still kills bashy (143)'
seq: 363
status: todo
priority: p2
labels:
    - posix
created: 2026-09-30T16:52:56.414753Z
---

Found by the Sprint 340 conductor 2026-09-30 (coreutils weave #12 merge gate). Repro: python3 sets SIGTERM to SIG_IGN, then execs '<shell> -c "kill -TERM $$; printf survived"': /bin/sh -> rc 0 'survived'; ~/.local/bin/bashy.real -> rc 143, no output. POSIX.1-2017 XCU 2.11: signals ignored on entry to a non-interactive shell cannot be trapped or reset - the shell must keep them ignored. Impact: coreutils cmds/env TestEnvIgnoreSignal fails whenever sh on PATH is the bashy shim (~/.bashy/shims/sh -> bashy.real), which fails weave's full-suite merge gate for unrelated coreutils runs (worked around with PATH=/bin:/usr/bin:$PATH). Fix in the sh engine / bashy startup: record the inherited disposition and never reinstall a Go handler for signals that were SIG_IGN at entry. Red/green: the python repro above.
