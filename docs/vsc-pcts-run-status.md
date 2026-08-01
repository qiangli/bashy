# VSC-PCTS conformance run — status

The official POSIX conformance work runs against **VSC-PCTS2016**, The Open
Group's POSIX Verification Suite for Shell & Utilities, under a time-limited
OSS license (agreement v1.4.OSS).

**Publishing consent: GRANTED 2026-07-16, EXTENDED 2026-07-29 (ticket #280298).**
§1 of the license forbids publicly disclosing Test Suite results without The Open
Group's prior written consent. We published tallies + assertion identifiers here
between 2026-07-04 and 2026-07-11 without that consent; on noticing we removed
them and requested consent. The Open Group granted permission to publish results
**"for the purposes of conformance work, limited to the relevant tests related
to the shell utility, other existing requirements unchanged"** — and, asked
whether that reached the utility programs too, **confirmed on 2026-07-29 that the
consent extends to publishing results of the utility test sets, again for
conformance-work purposes only and under the same otherwise-unchanged
conditions.**

- **Shell-scenario results (`shell_no12`, `sh_12`) are published below** under
  the 2026-07-16 grant.
- **Utilities-sweep results are published below** under the 2026-07-29
  extension.
- **Conformance-work purposes only.** These numbers belong in engineering
  records like this one; they are not launch or marketing copy.
- **Independent of consent and unchanged:** no "certified" / "passes the Open
  Group suite" claim, no Open Group mark/badge, and the suite is never
  redistributed.

The full run record — journals, harness internals, the resumable launcher
contract — is held privately by the maintainer; what is publishable under the
grant is reproduced here.

## Shell scenario results (published under the 2026-07-16 grant)

The POSIX `posix` **shell** scenario, run through our non-privileged TET harness
(`tcc` as a non-root tester; a from-source GNU Bash 5.3 SUT run through the
identical harness is the reference arm). Every bashy-only conformance bug found
in the campaign was fixed (including #643); the residual failures are **shared
with certified GNU Bash 5.3 under the identical harness** — i.e. they turn on the
build/filesystem environment or specify behaviour beyond what POSIX requires,
not on bashy's shell conformance.

- **`shell_no12`** (the shell scenario excluding the interactive `sh_12` set),
  journal `0197be`, 2026-07-08: **368 PASS / 5 FAIL** / 5 UNRESOLVED /
  33 UNSUPPORTED / 25 UNTESTED. That is +10 passes over the reproduced
  **358 PASS / 5 FAIL** baseline (journal `0090e`, 2026-07-07), fail count
  unchanged — the known residual family, fail set `{379, 421, 450, 458, 520}`
  (#379 is the GA11 ctime flapper; #379/#450 trade places across runs).
- **`sh_12`** (isolated — the interactive/job-control test set), journal `0198be`,
  2026-07-08: **43 PASS / 12 FAIL** / 5 UNSUPPORTED / 3 UNTESTED. The 12 fails
  are the declared-limitation trap/signal set, the same set the certified
  reference shell exhibits under this harness.

Reading: the shell scenario is at its residual floor — the only remaining
failures are ones a certified Bash 5.3 also produces here. These are the numbers
we would carry into a conformance statement's declared-limitations section.

Nothing about the earlier withholding was a claim of secrecy or of a bad result —
the campaign is going well. It was a licensing term we should have honored from
the start.

## Utilities sweep results (published under the 2026-07-29 extension)

The utility parts of the `posix` scenario (`posix_cmd` / `upe` / `sdo` / `xopen`,
100 tsets) through the same non-privileged harness, with a **per-tset 10-minute
cap** — a single part-level timeout does not work, because one slow tset will eat
the whole budget before the rest of the sweep runs.

Two arms, one harness. The **bashy arm** prepends our coreutils multicall binary
and its ~135 command symlinks to the suite's `PATH`, so PCTS scores bashy's
pure-Go tools wherever a name exists and the platform's GNU tools fill the rest.
The **GNU baseline arm** is the same sweep against the stock toolchain. So this
measures *bashy's userland against GNU's userland under identical conditions* —
not against a POSIX ideal, and not a certification result.

| arm | PASS | FAIL | UNRESOLVED | UNSUPPORTED |
|---|---|---|---|---|
| bashy userland (2026-07-08) | **2551** | **912** | 354 | 1341 |
| GNU baseline (2026-07-07) | 2947 | 516 | 392 | 1313 |

**86.6% of the GNU arm's pass count.** The +396 fail delta is **concentrated**,
not spread — six commands carry more than half of it:

- **sed +69, grep +59** — one root system, not two: the BRE/ERE engine
  (`pkg/bre`, in coreutils) plus sed's feature grammar. ~130 of the delta.
- **find +51** — `-exec` / `-ok` are on the NO-list (they shell out) and PCTS
  tests exactly those, plus primary edge semantics.
- **ls +20, expr +18, xargs +18, pr +17, env +16** (`env COMMAND` is likewise
  NO-list), **id +12, od +9, mkdir +8, rm +7**.
- ~35 further tsets at **+1..+6** — long-tail semantic edges.
- **At parity with GNU:** `getconf`, `getopts`, `true`, `false`, `time`, `who`,
  and the cap-limited `diff` / `ed` / `stty` / `more` / `crontab`.
- **Not comparable across arms** (excluded from any reading of the delta):
  `at` / `batch` / `tail` (per-tset cap artifacts differ between arms), `kill`
  (bashy's builtin answers via the shell, not the userland shim), `patch`
  (fixture collateral).

Also from the utility arm, bashy scored **as the `sh` utility** (the `sh` tset
inside `posix_cmd`, journal `0134be`, 2026-07-07): **41 PASS / 7 FAIL** /
192 UNSUPPORTED. The seven: GA26 (#5), `sh -s` (#46), PATH_MAX (#59), `-c`/`-s`
stdin handling (#67, #68), syntax-error-in-subshell exit status (#244), and
async-events default (#801). These have not been triaged into real-bug vs
declared-limitation vs environment.

Reading: the uutils-parity campaign closed the GNU **option surface**; PCTS
measures POSIX **runtime semantics**, which is a different axis — and it is the
userland's current conformance frontier. The two NO-list entries the delta
charges (~67 fails between `env COMMAND` and `find -exec`) are now
data-justified candidates for the command-wrapper exception rather than open
style questions.

⚠️ **Currency caveat — read these as the measurement that motivated the work,
not as today's score.** 2026-07-08 is the most recent *scored* utilities
baseline. The `pkg/bre` regex cluster has since closed (5 fixes + an ERE/BRE
parity lock), and **it has not been re-scored against PCTS** — that needs a Linux
SUT and the licensed harness. The sed/grep lines above are therefore stale in the
favourable direction, and the honest statement is that we do not yet know by how
much. A later serial remeasurement (2026-07-18) was run cert-shaped for feedback
discipline but produced no publishable per-command tallies, so it does not
supersede this table.

## What is (and isn't) constrained

- **Shell-utility test results — publishable** (per the 2026-07-16 grant):
  shell-scenario pass/fail tallies, assertion identifiers, reference-shell
  comparisons, for conformance-work purposes.
- **Utilities-sweep results — also publishable** (per the 2026-07-29 extension):
  anything derived from the utility (non-shell) tsets, on the same terms.
- **Not constrained — ours, and published as always:** every measurement made
  with our own or freely-licensed harnesses. The Bash 5.3 fixture suite
  (`make test-bash`), the yash POSIX scoreboard, the clean-room differential and
  10-shell panel (`scripts/oils-diff.sh`, `scripts/multishell-diff.sh`), and the
  POSIX-mode parity sweep. Those are the numbers in `docs/TODO.md` and
  `docs/conformance-statement.md`, and they are unaffected.
- Also unconstrained: the *fact* that a certification effort is under way, the
  harness scripts we wrote (`scripts/vsc-tet-build.sh`), and the declared
  limitations we intend to state in the conformance statement.

## Claim discipline (unchanged, and independent of the consent question)

bashy is **not** POSIX certified and does not claim to be. Certification is a
separate Open Group process — submission, conformance statement, declared
limitations — and only it confers the right to say "certified" or to use any
Open Group certification mark. Never write "POSIX certified", "passes the Open
Group suite", or an equivalent, anywhere.

## For maintainers

The suite itself is never committed to any repository (`.gitignore` enforces
this; the harness stages it outside the tree). The durable run record, the
harness runbook, and the campaign plan are held privately — ask the maintainer.

See `docs/bashy-v1.0.0-readiness.md` §License terms for the binding terms in
full: non-redistribution, the consent requirement, no certification trademarks,
the 12-month term, the destroy-on-expiry duty, and the feedback assignment.
