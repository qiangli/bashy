# Cross-shell conformance baselines (yash POSIX suite + the full-suite plan)

Status: **2026-06-27.** Baselines from running yash's POSIX (`-p`) suite against
bashy + 10 reference shells (`scripts/yash-posix-suite.sh`). These are the
**tracking baselines** for the future "100% on top of bash 5.3" / per-shell
compatibility work — NOT marketing figures (see Claim discipline below).

## Why the full suites matter (the lesson)

Our 43-case clean-room corpus reported **0 deviations**. yash's **1840-testcase**
POSIX suite found **435 cases bash 5.3 passes but bashy fails**. Small corpora
hide the truth — real baselines need the *full* upstream suites.

## Progress log

- **2026-06-27 — yash 90% + the `+i` unblock.** Full verified baseline:
  **`go test ./...` ALL PASS · `make test-bash` 86/86 (100%) · yash canonical
  90%** (1667 OK / 180 ERROR of 1847, signals/JC excluded) **· Oils drop-in
  fidelity 100%** (1103/1103 match, 0 diff vs bash:5.3). Canonical yash
  **78% → 90%** via the conformance sprint (cd/quote/param/trap/dot/command/simple
  clusters) **plus the `+i` CLI-flag fix** — bashy rejected `+i` as a script path,
  so the entire `sig*-p` disposition suite errored at startup (≈2160 spurious
  "gaps" that were a flag-parse bug, not the goroutine-not-fork ceiling). A
  baseline-run regression — sh `60ffa800` ("ENOENT for PATH-unset") being
  over-broad — was bisected and reverted. SHAs: sh `0e0f104f`, bashy `809f83a`.

- **2026-06-25 — invoked-as-`sh` → POSIX mode** (cli fix). Root cause: bashy
  didn't enter POSIX mode when argv[0] is `sh`, but yash's framework runs every
  test by invoking the shell *as sh* — so bashy ran them all non-POSIX and
  failed the POSIX-specific behaviors. One fix: **bashy 72% → 78%**, **bash-gap
  435 → 321** (`error-p` 171→112, `alias-p` 50→30), make test-bash held 86/86.
  Remaining gap leaders: error-p 112, umask-p 58, alias-p 30, option/quote/redir.

The numbers in the tables below are the **pre-fix baseline** (the starting point
the progress log measures against).

## yash POSIX (-p) suite — per-shell pass rate (1840 testcases)

Job-control/signal tests (sig*, bg/fg/job/kill/wait/testtty/async — need a TTY +
yash's checkfg helper) excluded uniformly; that's bashy's documented interactive-
JC limitation, equal for all shells.

| Shell | pass | | Shell | pass |
|---|---|---|---|---|
| yash (own suite) | 99% | bash 5.3 | 95% |
| mksh | 96% | dash | 91–95% |
| ash | 95% | bash 5.2 | 94% |
| loksh | 94% | ksh93 | 90% |
| zsh | 91% | **bashy** | **72%** |
| posh | 68% | | |

bashy sits above bare-minimal posh but ~20pts below the mainstream shells —
yash's relentless edge cases expose gaps our curated corpora never probed.

## Pairwise: bashy-vs-X verdict agreement (internal baseline)

How often bashy gives the same pass/fail verdict as each shell on the yash suite.
**None ≥85% on this torture-test metric** — it is the conformance-depth measure,
not a compatibility claim.

| bashy vs | agree | | bashy vs | agree |
|---|---|---|---|---|
| ksh93 | 78% | bash 5.3 / 5.2 | 76% |
| dash | 76% | ash | 74% |
| zsh | 73% | yash | 72% |
| mksh | 72% | loksh | 71% |
| posh | 61% | | |

## Bash-gap triage — the priority fix list (435 cases)

Cases **bash 5.3 passes / bashy fails** — closing these = matching bash on yash's
suite. **64% of the gap is in 3 files:**

| count | file | nature |
|---|---|---|
| 171 | `error-p` | error-condition handling — error-message *wording* (stderr; non-POSIX-mandated, cf. `plan-error-format-pass.md`) + some exit/control edge cases |
| 58 | `umask-p` | umask formatting/behavior |
| 50 | `alias-p` | alias semantics |
| 15 | `builtins-p` | misc builtins |
| 13 | `quote-p` | quoting |
| 12 | `option-p` | set options |
| (tail) | redir/cd/trap/set/simple/param/… | scattered |

Verified directly: most error *semantics* match bash (exit 127 on not-found,
subshell exit 3, readonly-var error); the divergence is concentrated in
error-message wording + a minority of exit/control edge cases.

## Claim discipline — the high numbers are inflated; anchor on the low one

**Corpus depth drives the number, and shallow corpora inflate it.** The same
bashy measures:

| Corpus | Depth | bashy "compatibility" |
|---|---|---|
| 43-case clean-room | shallow | 0 deviations (100%) |
| Oils (~1100) | medium | ~100% |
| multishell output-identical (43-case) | shallow | 93–100% per shell |
| **yash `-p` (1840)** | **deep** | **72%** |

The first three are **not** the real figure — they don't probe the edges. The
moment a deep suite runs, the truth is **72%** (with a 435-case gap to bash).
**So the honest anchor is the low, deep-suite number — never the inflated
shallow-corpus ones.** Treat every "9x%/100%" as *"on a corpus that didn't go
deep,"* the same way yash's suite revealed bashy is "really just 72%."

What we may state, all verifiable, none inflated:
- ✅ **"Passes bash 5.3's own fixture suite, 86/86."** (run `make test-bash` —
  exact, reproducible, a real headline.)
- ✅ **"Deep POSIX conformance is a work in progress: 72% on yash's 1840-case
  torture suite, closing a known 435-case gap to bash 5.3."** (honest WIP.)
- ❌ **Do NOT** pitch "93–100% compatible with dash/zsh/…", "100% POSIX", or any
  bare "N% compatible" — those ride the shallow corpora and overstate. Even
  *bash itself* is only 95% on yash's suite, so "100% anything" is corpus-relative.

The deep number isn't a bad-news story — it's the **honest baseline + the
roadmap**. The pitch is "a real bash 5.3 drop-in (86/86) that is rigorously and
measurably closing the deep-conformance gap," not a hollow "100%."

## Forward plan: run the FULL upstream suites from source

The yash suite is the best *cross-shell* one (its `-p` split runs any shell). To
get true per-shell baselines we run each shell's **own** full suite from source,
prioritized by signal:

| Priority | Suite | Signal | Status |
|---|---|---|---|
| 1 | **yash `-p`** | cross-shell POSIX (gold standard) | ✅ done (this doc) |
| 1 | **bash `tests/`** (full, not just our 86 fixtures) | our primary target | partial (86/86 fixture subset) |
| 2 | **os-test (Sortix)** | cross-shell/OS POSIX | TODO |
| 2 | dash, busybox-ash, posh | minimal-POSIX, low noise | TODO |
| 3 | ksh93 `tests/`, mksh `check.t`, zsh `Test/` | high noise (test their own extensions) | TODO |

Each non-yash suite needs a per-framework adapter (formats differ). `scripts/
yash-posix-suite.sh` is the template for the cross-shell-runnable ones. GPL
suites (yash, bash, …) are cloned at runtime into gitignored caches, never
vendored.
