# Cert-dash sh-engine findings (WS3 inbox)

Status: **live, 2026-06-27.** Concrete `../sh` engine bugs surfaced by the POSIX
cert dry-run (`scripts/posix-certdryrun.sh`). Each is a fix in the `qiangli/sh`
fork (`interp`/`expand`/`syntax`), gated by `cd ../sh && go test ./...` green +
`cd bashy && make test-bash` still **86/86**. Companion to
`yash-conformance-gap.md` (the 112-case yash delta). This file lists the
*specific, root-caused* bugs; the yash doc lists the clustered worklist.

## 1. `let --` parse-errors from a file — **FIXED** (sh `syntax/parser_arithm.go`)

Found by `scripts/modernish-suite.sh`. modernish's init-time fatal-bug self-test
(`lib/modernish/adj/fatal.sh`) contains `let --`; bashy parse-errored it when read
from a file. **Fixed 2026-06-27:** in `arithmExprLet` context a bare `++`/`--`
with no operand that ends the `let` word (newline/`;`/`&`/…) now emits an empty
operand and defers to a runtime arithmetic error — matching bash (which evaluates
`let` args as arithmetic at runtime) and the existing `_EOF` arm (which is why
`-c 'let --'` already worked). Gated: sh `go test` delta 0, `make test-bash`
86/86. Verified: `let --` from a file → runtime error, rc=1, execution continues
(was: parse error, exit 2).

## 1b. `kill -s 0 $$` rejected — **FIXED** (sh `interp/builtin.go`)

Found by the same modernish init (FTL_NOKILLS: `kill -s 0 $$ || exit`). bashy's
`kill -s` branch used a name-only signal lookup, so a numeric sigspec — including
`0`, the null signal used to check process existence — was rejected
("invalid signal specification"), while `kill -0` and `kill -n 0` already worked.
**Fixed 2026-06-27:** `-s` now uses `parseSignalSpecPosix` (number-or-name), same
as the bare `-NAME`/`-NUM` form. `kill -s 0/-s 9/-s TERM` all work; posix mode
still rejects SIG-prefixed names. Gated: interp `go test` delta 0, `make
test-bash` 86/86.

## 1c. modernish init — still a multi-bug gauntlet (TRACKED)

The two fixes above cleared modernish's first two fatal trips, but its
`adj/fatal.sh` runs ~250 lines of bug probes and **collects** a verdict at the
end ("Fatal shell bug(s) detected"), so at least one more check still trips for
bashy. modernish applies this same all-or-nothing init to every shell — it
rejects **dash and yash outright** too — so passing it fully is a deep grind, not
a single fix. Finding the remaining trip(s) requires instrumenting `fatal.sh`
(it does not name the failing check). Candidate probe lines to investigate next
(stderr observed, may be deliberate probes or real bugs — confirm each vs bash
5.3): line 78 `command -@` / FTL_ASGNBIERR (verified NOT a bug — bashy reverts
the temp assignment correctly), line 165 `command eval '('` / FTL_EVALERR, line
209 `test … =~ …`, line 213 `test 1XX -eq`. modernish stays an INFO suite; this
is not a cert gate. NOTE: the directly cert-relevant frontier is the **yash
112-case delta** (`yash-conformance-gap.md`), which does not depend on modernish's
init passing.

## (historical) original `let --` finding

Repro:
```sh
# via -c: runtime arithmetic error (acceptable, matches bash's error class)
bin/bash -c 'let --'                 # → let: arithmetic syntax error; rc=1

# from a FILE: PARSE error (the bug) — bash 5.3 parses it as a runtime error
printf 'let --\necho after\n' | …    # or: bin/bash file.sh
#   file.sh: line 1: syntax error near unexpected token `--'
#   file.sh: line 1: `let --'

# control: `true --` / `: --` from a file parse fine → the bug is let-specific
printf 'true --\necho after\n' > f.sh; bin/bash f.sh   # → after; rc=0
```

Diagnosis pointer: the two parse paths disagree — `-c` reaches `let`'s runtime
arithmetic evaluation (correct), but the file/script parse path treats `--` as an
unexpected operator token. `let` is parsed as an arithmetic command in mvdan/sh
(like `((`)); the fix is in `../sh` `syntax` (the `let` clause / arithmetic
word parsing) so that `let --` parses to a runtime arithmetic error on both
paths, matching bash. Narrow, high-leverage: one fix unblocks the full
389-test modernish suite.

bash reference: `let --` → bash evaluates the arithmetic expression `--` (pre-decrement
with no operand) → runtime error, `let` returns non-zero; it does **not** parse-error.

## 2. (reserved) — yash-suite root causes

The yash delta (112 cases) is tracked separately in `yash-conformance-gap.md`;
its two dominant root causes (error-p "assignment error in subshell" ×~36;
alias-p substitution positions ×~30) graduate into this file once a single
underlying `../sh` fix is identified and verified for each cluster.

## Non-bugs confirmed by the sweep (record, do not chase)

- **dash brace-less function body** — `login () exec login "$@"` (no `{ }`):
  bashy *accepts* it (tracks dash/ash), bash *rejects* it (6/8 on
  `dash-posix-suite.sh`). This is a deliberate ash-compatible superset, not a
  POSIX gap — POSIX requires the function body be a compound command, and both
  behaviors are defensible. Leave as-is.
- **Austin-Group corner cases** — `austin-defects.sh` 37/37 match bash 5.3
  `--posix` (IFS edges, parameter-expansion longest/shortest, arithmetic
  truncate-toward-zero + signed `%`, `&&` short-circuit, special-vs-regular
  builtin assignment persistence, `read -r`). No action.
