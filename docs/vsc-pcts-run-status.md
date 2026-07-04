# VSC-PCTS conformance run — live status + resumable recipe

Status: **harness built + running; 2 systemic bugs fixed; 19 fails remain.**
Updated 2026-07-04. This is the durable record of the official POSIX VSC-PCTS
run against bashy so the campaign resumes without re-deriving anything.

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
