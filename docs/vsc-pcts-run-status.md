# VSC-PCTS conformance run — live status + resumable recipe

Status: **campaign complete — every bashy-only conformance bug fixed (incl.
#643); all residual fails are shared with certified bash 5.3 in the identical
non-root harness.** Updated 2026-07-05. This is the durable
record of the official POSIX VSC-PCTS run against bashy so the campaign resumes
without re-deriving anything. Remaining: the final scored run (default timers,
sh_12/sh_13 isolated) + the human Open Group submission step.

## 2026-07-08 — shell holds/improves + FIRST utils sweep scored against the bashy coreutils userland

SUT rebuilt from bashy `e0b9bf6` + sh `4e89440c` + coreutils `d483e5e`
(the post-uutils-parity coreutils: option abbreviation, chcon, hidden
GNU spellings, --time words). Launcher `/vsc/run-night8.sh`; ledgers
`night8-progress` / `utils3-progress`, logs `/vsc/logs3/`, journals
`0197be`–`0298be`.

**Shell — no regression, slightly better.**
- shell_no12 (journal `0197be`): **368 PASS / 5 FAIL** / 5 UNRES / 33
  UNSUP / 25 UNTESTED — vs the 358/5 July-7 baseline: +10 passes, fail
  count unchanged (the known residual family, all shared with certified
  bash 5.3).
- sh_12 (journal `0198be`): **43 PASS / 12 FAIL** / 5 UNSUP / 3 UNTESTED
  — vs 42/12: +1 pass, the same declared-limitation trap/signal set.

**Utilities — first-ever scoring of OUR userland.** Prior sweeps scored
the container's Debian/GNU toolchain; this run prepended `/opt/cushim`
(the coreutils multicall + 135 symlinks) to the suite PATH, so PCTS
scored bashy's pure-Go tools wherever a name exists (GNU fills the rest).
100 tsets, same per-tset 10-min caps as the utils2 GNU arm:

| arm | PASS | FAIL | UNRESOLVED | UNSUPPORTED |
|---|---|---|---|---|
| bashy userland (utils3) | 2551 | 912 | 354 | 1341 |
| GNU baseline (utils2)   | 2947 | 516 | 392 | 1313 |

86.6% of GNU's pass count. The fail delta (+396) is CONCENTRATED — six
commands carry over half of it:

- sed +69, grep +59 (regex/text-engine depth: pkg/bre + sed feature
  grammar — one root system, ~130 of the delta)
- find +51 (`-exec`/`-ok` are NO-list + primary edge semantics)
- ls +20, expr +18, xargs +18, pr +17, env +16 (env COMMAND is
  NO-list; PCTS tests exactly that), id +12, od +9, mkdir +8, rm +7
- ~35 further tsets at +1..+6 (long-tail semantic edges); identical to
  GNU on getconf/getopts/true/false/time/who and the cap-limited
  diff/ed/stty/more/crontab
- not comparable: at/batch/tail (cap artifacts differ between arms),
  kill (bashy builtin scores via sh, not the shim), patch (fixture
  collateral)

Reading: the July uutils-parity campaign closed the GNU *option
surface*; PCTS measures POSIX *runtime semantics*, which is a different
axis — the userland's next conformance frontier. The NO-list "↻ revisit"
entries (env COMMAND, find -exec) are now data-justified under the
command-wrapper exception: PCTS charges them ~67 fails.

## 2026-07-07 — regression re-run: baseline reproduced with a fresh SUT

A fresh `cmd/bash` build (bashy `51dc0c2` + sh `4e89440c`) re-run through the
identical harness reproduced the final baseline: **358 PASS / 5 FAIL**, fail set
`{379,421,450,458,520}` — July 5's set plus #379, the documented GA11-ctime
flapper (#379/#450 trade places across runs). Zero new assertion fails; all
seven campaign fixes hold. Journal `0090e`.

**Launcher contract (learned the hard way — the July-5 launcher script was
deleted, and reconstructing it without these invariants silently measures the
wrong thing).** A cert-valid run needs ALL THREE:

1. **Run `tcc` as the non-root tester** (uid 1009 via `runuser -u tester`) —
   root silently invalidates every permission-based assertion.
2. **Prepend `/opt/vscbin` to `PATH`** (holds `sh -> /opt/sut/sh`). The tset
   sources invoke the inner shell as plain `sh script` resolved from PATH, NOT
   from `TET_EXEC_TOOL` — without this the suite scores `/usr/bin/sh` (dash)
   and reports a deterministic 14-fail set that looks like pre-fix bashy.
3. `TET_EXEC_TOOL=/opt/sut/sh` in `tetexec.cfg` (drives the `.ex` test-case
   scripts themselves).

Reference launcher: `/vsc/run-full.sh` in the `vsc-build` container (runs the
whole `posix` scenario in bounded parts — shell, sh_12 isolated, cmd, upe,
sdo, xopen — so a hang in sh_12's interactive TPs can't stall the rest).
Sanity check before trusting any journal: `head -1 <journal>` must show
uid 1009, and a quick probe that the SUT is what answers:
`runuser -u tester -- env PATH=/opt/vscbin:$PATH sh -c 'echo $BASH_VERSION'`
must print the bashy version string (dash prints an empty line). Wall-clock
is NOT a reliable tell (~70s quiet vs ~200s loaded, both under bashy).

## 2026-07-07 — first ENTIRE-suite sweep (shell + sh_12 + all utilities)

Beyond the shell scenario, the whole `posix` scenario ran for the first time,
as a per-tset bounded sweep (`/vsc/run-utils2.sh`, 10-min cap per tset — a
single part-level timeout does NOT work: the `diff` tset alone burned 3.5h of
a 4h budget). Results by part:

- **shell (sh_04–11+13), journal `0090e`**: 358 PASS / 5 FAIL — the July-5
  baseline reproduced with a fresh SUT (see the regression section above).
- **sh_12 isolated, journal `0091e`**: **completes in ~12 min — no hang**
  (the readline DSR-deadline fix appears to have unblocked the pty flow):
  42 PASS / 12 FAIL / 5 UNSUPPORTED / 3 UNTESTED. The 12 fails are the
  trap/signal/interactive/special-builtin-error family (assertions
  #709 #712 #714 #718 #720 #739 #753 #757 #759 #761 #762 #765) — the
  declared-limitation class, now enumerated precisely.
- **utilities (posix_cmd/upe/sdo/xopen, 100 tsets)**: sweep totals
  2947 PASS / 516 FAIL / 392 UNRESOLVED / 1313 UNSUPPORTED (plus the first
  17 cmd tsets from the part-mode journal `0093be`: 961/194). These score
  the container's Debian/GNU toolchain UNDER bashy as the orchestrating
  shell — context, not bashy's cert scope. Big fail/unresolved blocks are
  environment or GNU-vs-POSIX divergence: `bc` 106F (GNU bc grammar),
  `pax` 139F, `at` 89U (no atd), `ex`/`vi`/`more`/`talk` mass-UNRESOLVED
  (no tty). Five tsets hit the 10-min cap: `diff` (hangs in its TET BUILD
  phase — unexplained, makefile itself is instant), `ed`, `kill`,
  `crontab`, `fc` (fc is bashy's own builtin — worth a look).
- **`sh` tset inside posix_cmd (bashy as the `sh` utility), journal
  `0134be`**: 41 PASS / 7 FAIL / 192 UNSUPPORTED. The 7: GA26 (#5),
  `sh -s` (#46), PATH_MAX (#59), `-c`/`-s` stdin handling (#67 #68),
  syntax-error-in-subshell exit (#244), async-events default (#801).
  Not yet triaged real-bug vs declared vs environment — the next
  per-assertion analysis target.

Journals for the sweep live in the container at `/opt/tet/vsc/results/`
(`*be` = build+execute journals); per-tset logs in `/vsc/logs2/`; the
per-tset ledger is `/vsc/utils2-progress`.

## 2026-07-05 (final) — every bashy-only conformance bug fixed

The campaign was driven to completion by (a) fixing every real bashy bug and
(b) running the suite the way the cert intends — **as a non-privileged user,
not root**. The original config had `VSC_TESTER=root`, which silently
invalidates every permission-based assertion; switching to a `tester` uid via
the suite's own setuid priv-helpers is the correct, cert-valid setup.

**Seven bashy-only bugs root-caused, fixed, regression-tested, and confirmed
FAIL→PASS in the licensed harness** (`make test-bash` 86/86 throughout):
`#378` (redirect base mode 0666), `#396` (partly-quoted here-doc delimiter),
`#575` (literal trailing `[` in a glob component), `#621` (collision-safe
`exec` fd placement), `#671` (`set -e` on loop/case with `&&`-exempt tail),
`#352` (`$?` from a cmdsubst in a case/for word), `#643` (interactive shell
must not exit on a special-builtin error — `interp` interactive exemption +
a `readline` DSR-deadline fix so the prompt reaches a non-responding `expect`
pty). `#450`/`#611` resolved simply by running as non-root.

**The remaining fails are NOT bashy bugs — bash 5.3 (POSIX-certified) fails
every one of them in the identical non-root harness.** Verified by running
bash 5.3 as the SUT: its non-root fail set is `352 379 421 440 450 458 520`
(and bashy now *passes* `#440`/`#450`/`#352`, i.e. exceeds bash on some):

| # | Nature (bashy == or ahead of certified bash 5.3) |
|---|---|
| #379 | GA11 file-attribute ctime check needs `getconf _POSIX_TIMESTAMP_RESOLUTION` (unsupported here) → assumes 1s resolution and fails on sub-second ctime. Filesystem/`getconf` environment artifact, not shell behaviour. |
| #421 | `x=5 date` (readonly x, a *regular* command): POSIX 2.8.1 does **not** make a variable-assignment error in a non-special command exit the shell. bashy matches `bash --posix` exactly (both continue); the test is stricter than POSIX. Fixing it would diverge from bash. |
| #450 | `$0`/`ARGV[0]` of a slash-path command — correct in isolation (bashy prints the full path like bash); fails only intermittently under the harness (command-hash/`command -v` env). Shared with bash 5.3. |
| #458 | `&&`/`||` must not treat a `SIGTSTP`-stopped async child as exited — job-control/signal behaviour bash 5.3 also fails here. |
| #520 | async list (`&`, no `-m`) must ignore SIGINT/SIGQUIT in the child — bashy no longer core-dumps (former highest-severity crash fixed); now exits like bash 5.3 does, which also fails this. Making an external async child inherit SIG_IGN needs process-wide signal manipulation unsafe in Go's multithreaded runtime (the goroutine-subshell limit). |

Net: bashy is at parity-or-ahead of the certified reference shell on the POSIX
shell suite; the residual fails are declared deviations / environment artifacts
for the conformance statement, exactly the class the scenario already excludes
for `sh_12`.

## 2026-07-05 — differential campaign: 5 real bashy bugs fixed

Running bash 5.3 (built from source) as the SUT through the *identical* TET
harness separated real bashy bugs from suite/environment artifacts. Of the
bashy-only fails, **five deterministic bugs were root-caused, fixed, regression-
tested, and confirmed FAIL→PASS in the licensed harness** (`make test-bash`
stayed 86/86 throughout):

- **#396** — a partly-quoted here-doc delimiter (`<<''EOF`) must suppress body
  expansion; `unquotedWordBytes` clobbered its quotedness flag across word
  parts (fix: `syntax/parser.go`).
- **#671** — `set -e` wrongly exited on a for/while/until/case whose final body
  command was the errexit-exempt left operand of `&&`/`||` (fix: `interp/runner.go`).
- **#378** — file-creating redirects opened at base mode 0644 instead of bash's
  0666, dropping a umask-permitted group/other-write bit (fix: `interp/runner.go`).
- **#621** — `exec CMD N>file` left the redirect fd close-on-exec (and reshuffled
  fds in unspecified map order), so a descriptor never reached CMD; fix places
  them collision-safely with a private-fd pass (fix: `interp/os_unix.go`).
- **#575** — a literal trailing `[` in a globbing path component (`d*[` matching
  a directory named `dir[`) was mistranslated as an unterminated bracket
  (fix: `expand/expand.go`).

The remaining bashy-only fail is **#643**, an interactive `expect`/pty test
(readonly vars driven through a spawned interactive `sh`, matching a `$ ` prompt
after each line). It exercises the goroutine-subshell/readline interactive
surface — the same class the scenario already excludes for `sh_12` — and is a
**declared limitation** for the conformance statement, not a defect in the shell
language semantics. (A bounded partial improvement to the readline cursor-
position (DSR) query — which otherwise blocks forever on a non-responding pty —
was prototyped but not landed; the full interactive flow also needs a POSIX-mode
prompt default and pty-init hardening, tracked as future interactive work.)

All other remaining fails are **shared with bash 5.3** in the same harness, i.e.
suite/build-environment artifacts rather than bashy bugs.

---

### Original snapshot (2026-07-04, superseded above)

## What exists now (all working)

- **Licensed suite** downloaded + backed up: `~/vsc-pcts/testsuites` (VSC-PCTS2016-3.1
  + TET3.6-lite + VSXgen4.11), read-only, sha256-manifested, one-file backup
  `~/vsc-pcts/vsc-pcts2016-suite-pristine.tgz`. Re-downloadable from the Open
  Group SFTP server (host/account/keyword per the license email) — **only from
  the whitelisted host**: a static-IP cloud VM with a reverse-DNS PTR (a home/ISP
  IP is rejected). License = OSS v1.4: **never commit the suite; local/personal
  backups only.**
- **Build host:** a Mac Studio on the mesh — `bashy podman` container `vsc-build`
  (Debian, gcc-14), suite at `/vsc/testsuites`, SUT at `/opt/sut/sh` +
  `/opt/sut/bashy-real`.
- **Harness built in the container** (all reproducible):
  - TET3.6 built (`tcc` + `libapi.a`) with gcc-14 CFLAGS (`-fcommon` + demoted
    implicit-int errors) after `configure -t lite` + patchA.
  - VSC support tools built (`vrpt`, etc.); ImplSpec helpers built after adding
    an empty `libxnet.a` stub (glibc merged xnet); priv helpers setuid-root;
    `expect` installed.
  - `buildconf` (defaults: c99 + POSIX08, gcc-14 CDEFS) → `configure` fed the
    **57-line answer sequence** (recorded in `~/vsc-pcts/configanswers`) → run
    tree (`tetexec.cfg`, `tet_scen`).
  - SUT = bashy `cmd/bash` static linux/arm64, invoked POSIX via argv0=`sh`;
    `TET_EXEC_TOOL=/opt/sut/sh`.
  - Scenario: custom `shellscen` (`posix_shell` → `sh_04..sh_13`) to skip the
    undefined `posix_annexA/C` C/Fortran refs.
- **Run:** `bashy podman exec -d vsc-build bash /vsc/run-exec.sh` (detached);
  journal at `/opt/tet/vsc/results/NNNNe/journal`; tally via
  `awk -F'|' '$1==220{split($2,a," ");print a[3]}' journal | sort | uniq -c`
  (0=PASS 1=FAIL 2=UNRESOLVED 4=UNSUPPORTED 5=UNTESTED 7=NORESULT).

## Results (shell-language subset sh_04..sh_11 complete; sh_12 partial)

| | first run | after 2 fixes |
|---|---|---|
| PASS | 365 | **341** |
| FAIL | 48 | **19** |
| UNRESOLVED/NORESULT | 11 | 6 |
| UNSUPPORTED | 39 | 34 |

Determinate pass rate **95%** (341/360). The FAIL drop came from fixing two
systemic bashy bugs (below), which also removed a flood of spurious NORESULT.

## Fixed (committed, shipped)

1. **`trap` numeric signals 16-31** — used a stale 0-15 `signalNames` map;
   `trap ... 28/29/30/31` was rejected. Now uses `signalByNumber()`.
   `sh` commit `21c80d55`.
2. **SIGURG (23) leaking into traps** — Go's runtime async-preemption signal was
   caught by `signal.Notify` and forwarded to shell traps (3121 fires/loop),
   poisoning any harness that traps signal 23 (the VSC TCM does) → mass
   NORESULT. `enableSignalTrap` now skips SIGURG like it skips SIGCHLD.
   Same commit.
   Shipped: bashy pin `784171b`, umbrella `8758a59`.

## Remaining 19 FAILs (need per-assertion captured-output analysis)

Naive repros of these PASS — bashy matches bash on the obvious cases, so each
fails on a specific scenario only visible in the VSC assertion source + the
captured actual-vs-expected output. **Next step: re-run each failing test set
with TET output artifacts preserved, diff actual vs the `.eso`/`.exp` expected
file, then fix in `../sh`.**

- **$@ / param expansion + field splitting** — #298, #299 (exit 14/10 vs 1).
- **command substitution parse** — #352, #440.
- **redirection-error exit (special vs regular builtin)** — #419, #420, #517.
- **command name with slashes** — #450; **PATH file search** — #611.
- **pipeline stdout→stdin** — #453; **`$!` background pid** — #462.
- **`set -u` non-interactive exit** — #691 (want ≠0, got 0).
- **bracket-expression collation** `[. .]`/`[= =]`/hyphen — #538/539/543/548
  (possible RE2 pattern-engine gap → fix or declared limitation).
- **CORE DUMP** — #520: SIGINT/SIGQUIT not ignored in an async list (`&`
  without job control) → bashy dumps core. Entangled with the goroutine-subshell
  model; highest-severity.
- **GA11 file attributes** — #379 (possibly the priv-helper path, verify).

## Not yet run / declared-limitation territory

- **sh_12** (signals/traps/job-control) — completes its scriptable-trap TPs but
  **hangs on the interactive/job-control TPs** (expect-driven pty, `fg`/`bg`,
  interactive signal waits) — bashy's documented goroutine-subshell limitation.
  Must be run isolated with per-test bounding, or the interactive TPs classified
  as declared limitations (standard for the cert). **sh_13 didn't run** (sh_12
  blocked it) — run it separately.

## Path to "100% + certified"

1. Per-assertion capture-output analysis of the 19 (fleet-parallelizable) → fix
   the real ones in `../sh`; re-run as the regression gate.
2. sh_12/sh_13: run isolated; fix scriptable-trap fails; document the
   interactive/job-control TPs as declared limitations (§ conformance-statement).
3. Restore default VSC timers for the final scored run.
4. Certification = the human Open Group step: submit the journal + conformance
   statement (with declared limitations). External, not agent-completable.
