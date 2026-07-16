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

- **Shell-scenario results (`shell_no12`, `sh_12`) MAY now be published here.**
  *(Restoration of the specific tallies is pending — see the note below.)*
- **The coreutils/utilities sweep results remain WITHHELD.** "The shell utility"
  does not clearly cover the utility programs; a one-line scope follow-up on the
  ticket is needed before those are published (the utilities arm is the active
  campaign front, so it is worth confirming).
- **Independent of consent and unchanged:** no "certified" / "passes the Open
  Group suite" claim, no Open Group mark/badge, and the suite is never
  redistributed.

> **Pending:** the pre-removal record interleaved shell + utility results; the
> shell figures are being re-extracted cleanly (so no utility number is
> republished by accident) before they land back here. Full private record:
> `dhnt/docs/vsc-pcts/run-status.md`; grant: `dhnt/docs/legal/pcts-publication-consent-granted.md`.

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
