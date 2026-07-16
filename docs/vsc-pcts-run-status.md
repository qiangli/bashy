# VSC-PCTS conformance run — status

The official POSIX conformance work runs against **VSC-PCTS2016**, The Open
Group's POSIX Verification Suite for Shell & Utilities, under a time-limited
OSS license (agreement v1.4.OSS).

**Publishing consent: GRANTED 2026-07-16 (ticket #280298) — SCOPED.** §1 of the
license forbids publicly disclosing Test Suite results without The Open Group's
prior written consent. We published tallies + assertion identifiers here between
2026-07-04 and 2026-07-11 without that consent; on noticing we removed them and
requested consent. The Open Group has now granted permission to publish results
**"for the purposes of conformance work, limited to the relevant tests related
to the shell utility, other existing requirements unchanged."**

- **Shell-scenario results (`shell_no12`, `sh_12`) are published below** under
  the 2026-07-16 grant.
- **The coreutils/utilities sweep results remain WITHHELD.** "The shell utility"
  does not clearly cover the utility programs; a scope follow-up on the ticket is
  pending before those are published (the utilities arm is the active campaign
  front). No utilities tallies appear in this file.
- **Independent of consent and unchanged:** no "certified" / "passes the Open
  Group suite" claim, no Open Group mark/badge, and the suite is never
  redistributed.

Full private record (shell + withheld utilities): `dhnt/docs/vsc-pcts/run-status.md`;
grant: `dhnt/docs/legal/pcts-publication-consent-granted.md`.

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

## What is (and isn't) constrained

- **Shell-utility test results — now publishable** (per the 2026-07-16 grant):
  shell-scenario pass/fail tallies, assertion identifiers, reference-shell
  comparisons, for conformance-work purposes.
- **Utilities-sweep results — still withheld** pending the scope follow-up:
  anything derived from the utility (non-shell) tsets.
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
