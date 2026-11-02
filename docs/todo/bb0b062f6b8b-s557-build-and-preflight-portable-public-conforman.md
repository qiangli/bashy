---
id: bb0b062f6b8b
kind: bug
title: S557 build and preflight portable public conformance artifacts
seq: 3035
status: done
priority: p1
created: 2026-11-01T16:55:21.123166Z
assignee: s557-public-tooling
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
---

Free POSIX CI41327538807 produced bogus corpus deviations because posix-diff.sh built cmd/bash with inherited CGO1 then mounted the glibc-dependent binary into Alpine/musl. Use the existing product build wrapper and CGO_ENABLED=0 for portable Linux conformance artifacts; verify testee executes in each image before counting assertions. Preserve corpus, denominators, comparison semantics and raw failures. Audit same artifact prerequisite in related public runners. Short mocked builder/preflight checks local; actual suites remote only.

Delivery: posix-diff, modernish and dash producers use the product runtime build
wrapper with CGO disabled. Each executes the mounted testee inside its panel
image under a 10-second timeout before any assertions; failed startup exits 2
without reporting corpus measurements. Corpus contents and comparison rules
are unchanged.

Validation: bash syntax and diff checks passed. Six isolated mocked checks
verified Linux/amd64 CGO0 product builds, successful startup reaching the corpus,
and startup exit127 returning2 before corpus execution across all three scripts.
Actual container suites remain remote validation work.


Fresh hosted evidence: public validation run 41332727797 completed. Its Yash
panel measured Bashy 1778 OK / 55 ERROR of 1833; every Bashy ERROR is also an
ERROR for Bash 5.3, with zero Bashy-only errors and 18 Bashy-only passes. These
remain ERROR, not normalized passes. All 751 recorded fixture runner exits
were zero. Dash loaded 6/8 for both Bashy and Bash 5.3: login/newgrp use a
non-compound function body rejected by both. Modernish remains BLOCKED with
exit 128; the Bash reference measured 380 successes, 8 skips, and 1 warning.

Diagnostic follow-up: pinned modernish cde49fa37f93e943b9cb9b9536ce83836c4864ae
fatal.sh explicitly supports MSH_FTL_DEBUG (lines 37–41). The loader runs that
initialization before handling --version. After a blocked Bashy initialization,
retain separate Bashy and Bash reference --version traces with MSH_FTL_DEBUG=1
and timeout -s KILL 60. Do not rerun the corpus, modify upstream fixtures, or
change the original verdict. Actual diagnostic execution is hosted-only.

Diagnostic follow-up for run 41380385436: the separate initialization/version
attempt now exits zero, while the original self-test terminates with SIGALRM
and no summary. Opt-in MODERNISH_SELFTEST_TRACE=1 now retains a separate
60-second KILL-bounded self-test trace after the original verdict is recorded.
Loader -x traces the driver; test -xxx retains all per-test traces. Setup,
execution and trace-copy statuses are separate; original log/status/counters
remain unchanged. Hashbang module preload precedes loader xtrace activation,
so a failure during preload may still require a narrower diagnostic.
Only the explicit umbrella Modernish diagnostic lane enables this opt-in.
Four suite-free mocks passed: opt-out, diagnostic success, SIGALRM and timeout
statuses, retaining original evidence and copying completed-attempt traces.
Shell syntax and diff checks passed. No foreign suite or product build ran
locally; exact failure attribution and hosted validation remain pending.

Run 41381899105 reproduced the original SIGALRM result and created per-test
traces, but artifact upload failed with EACCES on a copied trace directory.
The diagnostic copier now grants read/traverse permission on its completed
artifact copies only and records that operation's exit status. It leaves
fixture and live-trace modes unchanged. The failed upload provided no raw
self-test trace, so exact signal attribution remains unresolved.

Diagnostic followup: the separate, opt-in 60-second Modernish self-test trace
now inherits descriptor 9 with BASH_XTRACEFD=9, retained as
bashy.selftest-xtrace.log. This keeps trace output visible when a capability
probe suppresses stderr. Original stdout/stderr, exit status, score and test
invocation are unchanged. Diagnostic copies, including the new file, receive
host-readable permissions; fixture/live trace modes are untouched.

Validation: shell syntax PASS; authored extracted-block mocks PASS with trace
opt-out and diagnostic exits 0, 142 and 137. The mocks verified descriptor 9
capture separately from stdout/stderr, exact timeout argv, readable copied
artifacts and unchanged original log/status. No foreign suite or product build
was run locally. Hosted evidence remains required; no runtime/PID change.

Windows redirection diagnostic followup (linked Sh story #187,
de21afd1cbc8): --redir-probe-dir adds a separate, explicitly unscored runner
invocation after normal private fixture/userland preparation. Run the original
`--tests redir` measurement first against the same retained candidate. The new
flag runs four authored CLI assertions and unchanged redir11.sub directly,
through THIS_SH, and separately with xtrace. Each phase has the existing
60-second execution bound and process-tree guard; raw combined stdout/stderr,
actual process status and timeout are retained without fixture normalization.
Only authored cases receive assertion results; upstream subfixture phases
remain raw diagnostic observations, never invented pass counts.

Candidate and extensionless THIS_SH binary hashes must match. All candidate,
userland and upstream input hashes are recorded and checked after every phase.
The evidence destination must be new; previous evidence is never overwritten.
No candidate rebuild or upstream fixture edit occurs between phases. Scored
fixtures reuse their unchanged execution body through a shared helper.

Bounded local verification: authored mock success/failure retention, wrong
nested binary rejection, previous evidence refusal and existing normal/idle
stdin/JSON runner guards passed. Windows amd64 test cross-compilation passed.
No foreign suite or product build was run locally. Native hosted execution and
Sh187 root-cause attribution remain pending; Windows84/86 is not reclassified.

Review hardening: every diagnostic phase starts with a fresh hermetic fixture
copy; candidate/userland bytes remain shared and fixed. An authored mock leaves
a sentinel file and fails if any later phase sees it. Flag conflicts fail before
preparation. The final six focused runner/mock checks passed in 3.450s.
