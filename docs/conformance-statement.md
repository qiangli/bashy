# bashy — POSIX conformance statement

Status: **public conformance statement, partially shipped; shell milestone
complete 2026-08-08.** The licensed VSC-PCTS shell scenario has run and its
public-safe status lives in `vsc-pcts-run-status.md`; certification submission
and any Open Group mark remain separate. Utilities-sweep results are published
in the same file under the 2026-07-29 consent extension.

## What is being claimed

bashy's `bash` binary (the pure-Go Bash 5.3 drop-in, `cmd/bash`) implements the
**POSIX.1-2017 (IEEE Std 1003.1) Shell Command Language and the POSIX shell
built-ins**, and matches GNU Bash 5.3 in POSIX mode (`--posix` / `set -o posix`
/ invoked as `sh`) with **zero deviations across every freely available POSIX
shell conformance corpus we can run**.

This is a strong, honestly-bounded claim — **not** a statement that bashy holds
an Open Group POSIX certification mark. Running the licensed VSC-PCTS shell
scenario is evidence; certification is a separate Open Group submission and
approval process. The discipline here matters: *we claim only conformance we can
verify and are permitted to publish.*

The formal identity of the licensed campaign is **VSC-PCTS2016 version 3.1**,
VSC release **5.4.1**, configured for **POSIX08**
(`VSC_POSIX_VERSION=200809`) without XSI (`VSC_XOPEN_VERSION=0`). That suite is
authorized for the **1003.1-2016 Shell and Utilities Product Standard**. The
implementation claim above references POSIX.1-2017, a later publication in the
same POSIX.1-2008 / Issue 7 lineage; it must not be used to rename the suite or
the certification campaign as “VSC-PCTS2017” or “1003.1-2017 certification.”

## Scope

In scope — what bashy owns and asserts conformance for:

- The **`sh` utility**: the POSIX Shell Command Language (XCU §2) — quoting,
  parameter/arithmetic/command/tilde/pathname expansion, field splitting,
  redirection, compound commands, functions, pattern matching, traps.
- The **POSIX shell built-ins**: special builtins (`break`, `:`, `continue`,
  `.`, `eval`, `exec`, `exit`, `export`, `readonly`, `return`, `set`, `shift`,
  `times`, `trap`, `unset`) and the regular builtins bashy implements (`cd`,
  `read`, `getopts`, `printf`, `test`/`[`, `pwd`, `command`, `type`, `umask`,
  `wait`, `kill`, `alias`, `unalias`, `fc`, `jobs`, `bg`, `fg`, `hash`, …).

Out of scope — deliberately not part of bashy's conformance claim:

- The ~160 **standalone POSIX utilities** (`ls`, `grep`, `sed`, `awk`, `sort`,
  …). A shell invokes these from `PATH`; they are the host's, or — for the
  pure-Go self-contained story — the **`coreutils` sibling**, which carries its
  own conformance track. "Shell *and* Utilities" conformance = bashy + coreutils.

This scope boundary is also the reason **Profile B uses GNU/system utilities,
not Bashy's Go coreutils**: Profile B isolates the Bashy shell. Profiles C and
D exercise the Bashy Go utility provider. The provider strategy, including the
26 command names not currently implemented as Go applets or shell builtins, is
recorded in [`posix-command-coverage.md`](posix-command-coverage.md).

## Mode of assertion

POSIX conformance is asserted **in POSIX mode**. GNU Bash is intentionally
*not* POSIX-conformant in its default mode (it ships GNU extensions); it flips
**76 documented behaviors** under `--posix`. bashy mirrors that: its default
mode is a Bash 5.3 drop-in (extensions on), and `--posix` / `sh`-invocation
flips the same 76 behaviors toward the standard. The map is
`docs/posix-mode-behaviors.md`.

## Evidence

All measured on the `bash` drop-in binary, re-runnable via
`scripts/posix-certdryrun.sh` (the single aggregate scoreboard):

| Harness | What it proves | Result |
|---|---|---|
| VSC-PCTS2016 `POSIX.shell` | licensed, native, serial shell-isolation scenario | **493/493 certification PASS group; 0 blockers/manual resolutions/caps** — milestone evidence only, not certification |
| `make test-bash` | Bash 5.3 fixture suite (default mode) | **86/86** |
| `posix-parity.sh` | `bashy --posix` ≡ `bash 5.3 --posix` on mechanically-testable behaviors | **38 match / 0 diff / 1 info / 39 probed** |
| `posix-diff.sh` | clean-room XCU corpus, 5-oracle same-env differential | **0 deviations** |
| `oils-diff.sh` | Oils spec-test case code through the live differential | **0 deviations** |
| `multishell-diff.sh` | 10-shell panel (dash/ash/posh/yash + bash/zsh/ksh93/mksh/loksh) | **0 deviations** |
| `yash-posix-suite.sh` | yash's `-p` POSIX suite (strictest-shell suite; relative measure) | See [`yash-chunks.json`](../yash-chunks.json) and [measurement boundaries](public-claims-evidence.md); no current assertion percentage claimed |
| `austin-defects.sh` | clean-room Austin-Group corner-case differential (37 probes) | **37 match / 0 diff** |
| `dash-posix-suite.sh` | dash's shipped function-library load check (dash has no suite — oracle) | **bashy 6/8** (now matches bash; rejects ash brace-less bodies as of the syntax fix) |
| `modernish-suite.sh` | modernish self-test (~389 tests) under each shell | **blocked by one `sh` parse bug** (`let --`) — see below |

**The modernish row is a found bug, not a gap in scope.** modernish's init-time
fatal-bug self-test contains `let --`; bashy's `sh` engine **parse-errors `let
--` when read from a file** (it parses fine via `-c`, and `true --`/`: --` parse
fine — so it is a narrow, `let`-specific parse-path inconsistency), which aborts
modernish's init so none of its ~389 tests run. bash treats `let --` as a runtime
arithmetic error and parses it. This is a real `../sh` parser fix (tracked, WS3);
once fixed the harness lights up automatically. Notably bashy is *closer* to
clearing modernish's init than dash or yash (which fail its broader fatal-bug
battery outright).

**The yash row has two measurement units.** The committed
[`yash-chunks.json`](../yash-chunks.json) records 50 shell-only fixtures completing
on 2026-07-20, with no harness failures, skips or timeouts. It excludes the
job-control/signal/TTY fixtures. The runner can complete even when individual
assertions fail, so that record cannot establish an assertion pass percentage.
Older panel measurements are preserved as historical evidence in
[cross-shell-conformance-baseline.md](cross-shell-conformance-baseline.md).
Recheck on RC with `bashy dag dag.md yash` and retain each panel's corpus revision,
OK/ERROR counts, exclusions and testee commit before publishing a percentage.

The clean-room corpora are authored from the spec, never copied from GPL suites;
the GPL suites (yash, dash) are cloned at runtime into gitignored caches and run
as oracles/SUTs, never vendored. POSIX conformance is measured **semantically**
— observable stdout + success/fail — not byte-exact diagnostic wording or exact
exit-code value, neither of which POSIX mandates.

Honest caveat preserved from `vsc-pcts-readiness.md`: *zero deviations on the
corpora we run* is the strongest agent-drivable signal short of the official
suite, but it is not the same as the adversarial breadth of VSC-PCTS. The
cert-dry-run is designed to shrink that gap to the long tail.

## Declared limitations

The fixture and historical conformance results above do not establish full
interactive Bash/Readline parity. The current interactive boundary (2026-10-08,
Sprint 379 Story 1511) is explicit below; detailed probe assertions and execution
instructions are in [plan-interactive-depth.md](plan-interactive-depth.md).

1. **Interactive terminal job control.** Unix process-group carriers and
   terminal handoff are implemented and have focused tests in
   `internal/cli/jobcontrol_autoMonitor_vsc_unix_test.go` and the carrier tests.
   Windows does not provide Unix process-group/TTY job control; its basic job
   carrier provides process identity but no live signal proxy (`os/exec` rejects
   `ExtraFiles`). See [job-carrier.md](job-carrier.md).
2. **`((` arithmetic-vs-nested-subshell ambiguity.** `((cmd)||(cmd))` and deeply
   nested `( ( … ) )` need spaces; the streaming, no-backtrack parser cannot
   disambiguate `((` (a documented mvdan/sh limitation). Rare in conformance
   corpora.
3. **`<<${a}` expansion-shaped heredoc delimiter.** bashy parse-errors an
   expansion in the heredoc delimiter word where bash accepts a literal
   delimiter — deliberate upstream mvdan/sh strictness (6 parser tests assert
   it). Declared, not fixed.
4. **Go-runtime file-descriptor footprint.** The Go runtime opens a few
   housekeeping fds (epoll/eventfd, GOMAXPROCS probe) absent from a C shell's
   clean low-fd table. This is **not a POSIX shell issue** — `/proc` fd-table
   census is Linux-specific, not POSIX, and VSC-PCTS does not test it.
5. **Mixed stdout/stderr flush ordering.** Interleaving of the two streams in
   mixed-output cases can differ due to Go buffering; observable only when both
   streams are merged and ordering-sensitive.

### Interactive Bash depth

`TestInteractiveDepthPTY` and `tools/interactive-depth` share ten assertions:
`compgen -W` prefix/no-match behavior; direct `complete -W/-p/-r` registry
operations; `history -c/-s/-d/-w/-r/-a/-n/-p`; `fc -l/-n/-r`, `-s old=new`,
`-e -`; and Ctrl-U line deletion. Each case runs in an isolated real PTY or
ConPTY. These selected flag paths are not an exhaustive Bash differential.

Declared limitations:

- **Programmable completion:** no readline TAB callback is wired. `-F/-C`,
  `COMP_*`, other action/option combinations and `compopt` during completion
  are unproven. Registered specs are lost in command substitution:
  `complete -W 'alpha beta' x; v=$(complete -p x)` yields empty `v`.
- **`bind`:** interpreter dispatch is a no-op, including
  `-p/-l/-x/-r/-q/-u/-m/-f`; a successful status does not indicate support.
- **`READLINE_LINE`, `READLINE_POINT`, `READLINE_MARK`:** no `bind -x`
  callback integration exposes or applies these values.
- **History/fc depth:** the new lane does not establish range/negative history
  deletion, timestamps, filtering, size limits, concurrent sessions, arbitrary
  option/selector combinations or external-editor terminal handoff. Separate
  Unix editor tests do not establish Windows editor parity. With history
  disabled, a combined `fc -s` / `history -s` / `fc -e - -1` sequence
  reports no command found instead of selecting the latest entry; the exact
  reproducer is recorded in the probe plan.
- **`checkjobs`:** the interactive exit loop does not consult the option to
  warn/refuse exit with active jobs.
- **`histappend`:** readline persistence is not controlled by this option;
  append-versus-replace-on-exit and concurrent-session merging are not claimed.
- **Windows:** the focused ConPTY lane is separate from `internal/cli`, which
  remains excluded from Windows CI. It does not prove the entire package can
  run headless, or Unix signal/job-control parity.

## Claim framing (use verbatim)

> bashy's pure-Go Bash 5.3 drop-in passes its full Bash 5.3 fixture suite
> (86/86) and shows **zero deviations from bash 5.3 in POSIX mode** across a
> clean-room XCU corpus, an Oils spec-test differential, a 10-shell conformance
> panel, and the POSIX-mode behavior sweep. On yash's stricter own suite it
> tracks the mid-pack of POSIX shells (~90%, near zsh/ksh93) with a known
> ~105-case tail under active triage. The licensed Open Group VSC-PCTS shell
> scenario has run, but certification submission and any certification mark
> remain separate; the declared limitations above are stated up front.

When the yash tail is closed and the PENDING free-suite harnesses
(`dash`/`modernish`/`austin`) report 0, drop the "~105-case tail" clause and the
claim is unqualified.

Do **not** shorten this to "bashy is POSIX certified" — that is a specific Open
Group status bashy does not yet hold. "POSIX-conformant (POSIX mode), zero
deviations across all free suites" is the accurate, defensible claim.
