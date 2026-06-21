# VSC-PCTS readiness — cert-run pre-flight for bashy

Status: **pre-flight (2026-06-21).** Companion to `plan-posix-conformance.md`
(strategy/scope) — this doc is the concrete checklist + current-evidence
snapshot for an actual VSC-PCTS run. It does not repeat the conformance
strategy; read that plan first.

## What VSC-PCTS is

VSC-PCTS (The Open Group's POSIX Verification Suite for Shell & Utilities) is
the **official conformance test suite** behind POSIX/UNIX certification. It runs
under **TET** (Test Environment Toolkit) and is the authoritative oracle —
unlike our Oils/clean-room differential, which samples behavior against live
shells. Acquisition is a **license application to The Open Group** (an OSS/no-
cost 12-month arrangement has historically existed); **this is a human/legal
step, not something the agent can do.** Verify the current license terms at
application time.

## Scope (locked)

`sh`-utility conformance only: the shell command language + POSIX **built-ins**
(`cd`, `read`, `getopts`, `printf`, `test`/`[`, `export`, `set`, `trap`, `wait`,
…). The ~160 standalone utilities (`ls`/`grep`/`sed`/`awk`/…) are **out of
scope** for bashy — they belong to the host or, later, the `coreutils` sibling.
**Configure the TET scenario to the shell + builtins, or the run mostly tests
the wrong binaries.** Run bashy in **POSIX mode** (invoked as `sh`, or
`--posix`/`set -o posix`) — that is what VSC-PCTS exercises.

## Current evidence (where we stand)

Strong foundation — necessary, not yet proven-sufficient against the cert:

- **`make test-bash` 86/0** — full bash 5.3 fixture suite, green (default mode).
- **5-oracle same-env differential** (`scripts/posix-diff.sh`, oracles: bash 5.3
  / dash / yash / mksh / zsh) and the **Oils live-differential**
  (`scripts/oils-diff.sh`): **0 deviations** on the clean-room XCU corpus
  (`test/posix-corpus/`); 100% vs bash & zsh on it.
- **Gate-D mining: ~106 of 222 Oils suites → 12 real bugs found, ALL FIXED**
  (break-in-loop-condition, `cd --`, `printf %#x 0`, getopts mode, `printf %d`
  lone-quote, `$?` line-continuation, `[ '(' ]`, set -e brace-group exemption,
  tilde in `${:+}`, tilde `~user:` boundary, declare-family value re-expansion,
  redir-order expand-before-redirect). Each gated 86/0 + regression-tested.
- **POSIX-mode parser fix** (the `PosixMode` sub-flag) so `--posix` keeps bash
  grammar while flipping the quote/expansion rules.

Honest caveat: 0 deviations is on **our** sampled corpus. Gate-D found 12 bugs in
~half the Oils suites; the cert is adversarial and broader. "Clean on what we
tested" ≠ "conformant."

## Known limitations to DECLARE up front

Two areas are understood, bounded, and should be stated in the conformance
statement (and measured, not assumed, once TET is running):

1. **Interactive terminal job control** — `fg`/`bg`/Ctrl-Z/monitor-mode
   notifications are non-functional (goroutine-not-fork model). *Scriptable* job
   control (`wait`/`wait %n`/`$!`/`kill %n`/`jobs`) is ~conformant (Gate C:
   11/12). Plan of record to lift it: opt-in real-process path
   (`sh/plan-dual-mode-job-control.md`), to be built **only if** VSC-PCTS data
   shows interactive JC is load-bearing in batch mode.
2. **`((` arithmetic-vs-nested-subshell ambiguity** — `((cmd)||(cmd))` /
   `( ( (…) ) )` need spaces; the streaming no-backtrack parser can't
   disambiguate (documented mvdan/sh limitation). Rare in conformance corpora.

One open **decision** (not a limitation): `<<${a}` heredoc delimiter — bashy
parse-errors an expansion in the delimiter word; bash accepts. Decide whether
bash-fidelity warrants relaxing the parser before the run.

## Pre-flight checklist (in order)

- [ ] **Finish the conformance plan's Phase 0–1** (scope doc + POSIX-mode
      baseline) per `plan-posix-conformance.md`.
- [ ] **POSIX-mode breadth sweep** — our 86/0 is *default* mode; add a
      posix-mode fixture/differential pass (`scripts/posix-parity.sh` is the
      seed) so we are not blind to posix-mode-only behavior the cert hits.
- [ ] **Finish Oils mining** (remaining ~116 suites) → drive the differential to
      stable 0-deviations across the whole corpus; fix what surfaces.
- [ ] **Resolve the `<<${a}` decision.**
- [ ] **Apply for the VSC-PCTS license** (Open Group; human step). Confirm
      current terms + deliverables.
- [ ] **Stand up TET + wire bashy as the SUT** in POSIX mode (`sh` invocation);
      scope the scenario to shell + builtins.
- [ ] **Dry-run; triage results** into: real bug (fix, gated 86/0) /
      declared-limitation (JC, `((`) / scope-excluded (utility).
- [ ] **Decide dual-mode JC P1+** based on measured interactive-JC coverage.

## Go / no-go

**Go** when: posix-mode breadth sweep green, Oils mining at stable 0-deviations,
`<<${a}` decided, and the declared-limitations list is final. Pulling the
12-month license **before** these wastes the clock — the cert run should be
confirming a known-good state and catching the long tail, not discovering
basics. The shell-language core is in good shape; the gating work is breadth
(posix-mode + remaining suites) and the harness/license setup, not the language.
