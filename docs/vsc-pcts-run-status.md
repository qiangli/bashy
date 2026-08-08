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

## 2026-08-08 — POSIX shell-isolation milestone

**Lifecycle status:** shell-closure sprint #45 is complete. No shell
certification failures remain assigned or in progress; the continuing
certification work belongs to the broader Shell-and-Utilities campaign.

The complete 493-TP `POSIX.shell` scenario ran natively and serially on the
dedicated Ubuntu x86_64 test host. Bashy's Go applets were excluded so the arm
measured the shell language, builtins, and shell-to-system-command behavior;
the provider manifest proves that representative Coreutils commands resolved
from GNU Coreutils 9.11 rather than Ubuntu 9.4 or Bashy's multicall binary.

| metric | result |
|---|---:|
| configured shell TPs | 493 |
| certification PASS group | **493** |
| certification blockers | **0** |
| manual resolution required | **0** |
| FAIL / UNRESOLVED / UNREPORTED / INSPECT | **0 / 0 / 0 / 0** |
| caps | **0** |
| wall time | 855 s |

Coordinates: Bashy `1e3a14be`, sh `beb0b465`, harness `10c2ab5d`, GNU
Coreutils 9.11 at `/opt/gnu-coreutils-9.11`; arm
`bashy-shell-gnu911-pass-20260808T024000Z`. The source/evidence marker is
`vsc-pcts-posix-shell-2026-08-08`.

This closes the **shell-isolation engineering milestone**, not the Product
Standard or certification program. `UNSUPPORTED`, `NOT IN USE`, and `UNTESTED`
are included in the certification PASS group only under the declared profile
and retained rationale. The next milestone is all 116 Commands and Utilities
sets (8,844 configured TPs), with Bashy's Go utilities as SUT and GNU
Coreutils 9.11 as matched control. The assembled-provider shell rerun is a
regression gate within that campaign. One uninterrupted 117-set/9,337-TP
assembled-profile run, the suite-generated report, and the human Open Group
submission follow.

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

## 2026-08-01 — full re-measure, both arms, one x86_64 host

The 2026-07-08 figures below were scored on arm64 and pre-date the `pkg/bre`
regex work. This run re-scored **both halves of the product** (shell +
utilities) and **both arms** (bashy userland vs the stock GNU toolchain) on a
single disposable Linux host, so the comparison carries no architecture
variable. SUT is tag `vsc-pcts-2026-08-01`. Full POSIX08 scenario, 121 test
sets per arm, per-tset 10-minute cap.

| arm | PASS | FAIL | UNRESOLVED | UNSUPPORTED | UNTESTED | wall |
|---|---|---|---|---|---|---|
| bashy userland | 5,166 | 2,073 | 591 | 1,528 | 846 | ~3h |
| GNU baseline | 5,625 | 1,837 | 756 | 1,559 | 876 | ~2h |

**The result is the delta, not the fail count: +313 attributable to bashy.**
Both arms run identical tests, so most volume cancels — `m4` 92, `bc` 214
across three sets, `chown` 76, `make` 60, `ctags`/`strip` 24 each fail
*identically in both arms*, being utilities our multicall does not ship (both
arms scored the same distro binary) or environment properties.

**Per-assertion baseline: 291 bashy-only failing test purposes** across 63 sets
(bashy 1,602 failing TPs, GNU 1,381, of which 1,311 are shared and therefore not
ours). The full list — every TP with the suite's own assertion text, classified
into 12 parallel work streams — is
[`vsc-pcts-baseline-2026-08-01.md`](vsc-pcts-baseline-2026-08-01.md), and it is
the **regression baseline**: a later run reproduces those 291 or explains each
difference.

Remaining bashy-attributable failures by test set:

```
find +40   sed +35   date +34   dd +20   cp +16   pr  +15   patch +13
env  +12   ls  +11   awk   +7   rm  +6   cut +6   cat  +6   xargs  +5
od    +5   mkfifo +5  then +4/+3/+2/+1 across ~45 further sets
```

`sh_05 +2` is the only shell set with a real delta — **the shell arm is
essentially at parity with the reference.**

**The regex work landed.** Against the arm64 figures:

| tset | 2026-07-08 | 2026-08-01 | |
|---|---|---|---|
| grep | +59 | **+3** | essentially closed |
| expr | +18 | **0** | closed |
| id | +12 | +1 | closed |
| mkdir | +8 | +1 | closed |
| xargs | +18 | +5 | large improvement |
| sed | +69 | **+35** | halved |
| ls | +20 | +11 | halved |
| find | +51 | +40 | modest |
| **total** | **+396** | **+313** | |

`sed` halving rather than closing indicates the remainder is sed's own feature
grammar, not the shared BRE/ERE engine.

Three caveats, stated because they change how the numbers should be read:

- **`date +34` is new** — absent from the arm64 list entirely (GNU fails 2,
  bashy 36). Either a regression or newly-exercised coverage: this host
  provisions a `de_DE` locale and `date` is locale-dependent, so those tests
  could not previously run. Untriaged.
- **`at`, `diff`, `tail` are excluded** as not comparable — they hit the
  10-minute cap under bashy but not under GNU, so their bashy fail counts are
  *truncation, not success*.
- **The arm64 comparison is directional only.** The architecture changed *and*
  the denominator moved: provisioning a locale, read-only and no-symlink
  filesystems, and real special files means tests that were formerly skipped
  now run.

### A conformance defect the suite found

`$!` after backgrounding a **subshell group** returns an internal job label
rather than a PID:

```
$ sh -c '(:)& echo $!'        →  g1      (bashy)   66044 (dash)   66046 (bash)
$ sh -c 'sleep 0 & echo $!'   →  66042   (correct — a real process was exec'd)
```

A background job that never execs has no OS PID, so `$!` falls back to a
`g<N>` sentinel; real shells fork for `( … ) &`. This is not academic — TET's
own shell API uses `` tet_context=`(:)& echo $!` `` to mint a context id, so
`test $tet_context -eq 0` errored ~191 times per arm. Verdicts and assertion
diagnostics were unaffected.

## Utilities sweep results (2026-07-08, arm64 — superseded by the run above)

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

✅ **Superseded 2026-08-01.** These were the numbers that motivated the
`pkg/bre` work; that work has now been re-scored on x86_64 (see the section at
the top of this file) and the regex cluster is confirmed closed — grep +59 → +3,
expr +18 → 0. Keep this section as the historical arm64 baseline; quote the
2026-08-01 figures as current. A later serial remeasurement (2026-07-18) was run cert-shaped for feedback
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
