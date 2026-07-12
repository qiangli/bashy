# VSC-PCTS conformance run — status (results held privately)

The official POSIX conformance work runs against **VSC-PCTS2016**, The Open
Group's POSIX Verification Suite for Shell & Utilities, under a time-limited
OSS license (agreement v1.4.OSS).

**The run results are not published here.** §1 of that license forbids publicly
disclosing results of any use of the Test Suites, or claiming a product has
"passed" them, without The Open Group's **prior written consent**. We published
per-run tallies and assertion identifiers in this file between 2026-07-04 and
2026-07-11 without having sought that consent; on noticing, we removed them and
asked The Open Group for consent to republish. **This file will carry the
results again if and when consent is granted.**

Nothing about that is a claim of secrecy or of a bad result — the campaign is
going well. It is a licensing term we should have honored from the start.

## What is (and isn't) constrained

- **Constrained — not published without consent:** anything derived from running
  the licensed suite. Pass/fail tallies, journal data, the identifiers of
  individual assertions, and comparisons of suite results against reference
  shells.
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
