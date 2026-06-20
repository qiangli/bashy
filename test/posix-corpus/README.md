# POSIX XCU (Shell Command Language) conformance corpus

This is bashy's **POSIX shell conformance** corpus, driven by `scripts/posix-diff.sh`
(a 5-oracle differential harness). It is the shell analog of a POSIX test suite.

## Why not "the Open POSIX Test Suite"?

The well-known *Open POSIX Test Suite* (in LTP, `open_posix_testsuite/`) tests the
POSIX **System Interfaces** — the C API: pthreads, signals, semaphores, timers,
message queues. It is all C programs and exercises libc/the kernel, **not a shell**.
It would never run bashy.

Shell conformance is the POSIX **XCU "Shell Command Language"** volume. The
*official* certification suite for it (VSC-PCTS / The Open Group) is commercial and
not freely redistributable. So this corpus is **clean-room**: each case is written
from the POSIX standard's documented behavior (no copying from GPL suites like
yash's `tests/`), organized by XCU section. Apache-2.0 corpora (e.g. the Oils
`spec/` tests) may be adapted later for breadth, with attribution.

## How it works

`scripts/posix-diff.sh` builds one container image — `localhost/posix-shells`
(`bash:5.3` + `apk add dash yash zsh mksh`) — cross-compiles a Linux `bashy`, and
runs every `*.sh` here through all of them **in that one byte-identical environment**
(same busybox coreutils, same `$HOME`), file-arg, fresh cwd. That isolates *shell*
behavior from platform noise (e.g. BSD-vs-GNU `wc` padding).

Classification vs the oracle consensus:

- **MATCH** — bashy agrees with every oracle.
- **DEVIATION** — bashy disagrees where ALL oracles agree → a real conformance bug.
- **AMBIGUOUS** — oracles disagree among themselves (a bash/ksh extension or
  unspecified behavior); annotated with which oracle(s) bashy matches.

Oracles: `bash 5.3` (the drop-in target), `dash` + `yash` (strict POSIX), `mksh`
(Korn — shares bash's extensions, disambiguates "bash-only" vs "ksh-lineage"),
`zsh --emulate sh` (emulated POSIX). `csh`/`tcsh` are **excluded**: not POSIX
shells (a different language; cannot parse the corpus).

## Authoring rules

- **Pure POSIX** in `xcu-*.sh` files (no bash extensions) so all five shells agree
  and a bashy divergence shows up as a real `DEVIATION`. Put extension probes in
  separate files (they land in `AMBIGUOUS`, which is informative, not a bug).
- **Deterministic + hermetic**: no `$$`/`$!`/`$RANDOM`/dates/PIDs; use cwd-relative
  paths (the harness gives each run a fresh cwd); avoid platform-varying tool output.
- Name `xcu-*.sh` files by XCU section, e.g. `xcu-2.6-param-expansion.sh`.

## Sections covered (`xcu-*.sh`)

2.2 quoting · 2.5 special parameters · 2.6 word/parameter expansion ·
2.7 redirection · 2.9 compound commands · 2.11 trap · 2.13 pattern matching ·
2.14 special built-ins · `test`/`[` operators · POSIX arithmetic ·
command substitution. (Plus earlier ad-hoc behavior probes.)

Current: **0 deviations**; bashy vs bash 5.3 and vs zsh = 100%; the only AMBIGs
are bash/ksh extensions (`${v:off:len}` substring, `base#n` arithmetic) that strict
dash/yash reject by design.

## Run

```sh
bash scripts/posix-diff.sh            # requires a container runtime + Go
```
