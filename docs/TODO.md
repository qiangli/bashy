# Bashy: Bash 5.3 Drop-In Replacement — TODO Checklist

**Current status**: 🎉 86 bash tests passing, 0 failing, 0 skipped (of 86 measured fixtures) — **100% bash-5.3 compliance**
**POSIX frontier**: yash `-p` conformance suite **96%** (confirmed 2026-07-01 on novicortex; ≥ bash 5.3/5.2, tied with mksh for best of the 10-shell panel) — run `bashy dag dag.md yash`; details in `docs/cross-shell-conformance-baseline.md` + `docs/yash-conformance-gap.md`

**VSC-PCTS UTILITIES campaign (the active cert front, 2026-07-17)** — utilities-suite results remain withheld pending scope consent. Public updates may cite our own code changes and freely licensed harnesses only; no VSC-PCTS utilities tallies, assertion identifiers, raw journals, or private run-record pointers belong in this public repo.
  - **`pkg/bre` regex cluster CLOSED (sed+grep): 5 flips + 1 parity lock**, all in `../coreutils/pkg/bre`, each independently gate-verified: `RE_DUP_MAX` intervals (`65dce2e`) · bracket validation (`b3de4d3`) · anchor parity (`b5fbf7a`) · back-ref edges (`7bc67cd`) · collating/equivalence classes (`4dca9f6`) · ERE/BRE operator lock (`b852d57`).
  - **In flight**: `expr` (`cmds/expr`). **Next**: `ls/xargs/od/mkdir/rm` (non-NO-list). **PENDING USER DECISION**: `find -exec` is a NO-list reversal — do not implement without explicit go-ahead.
  - **Stewardship handed to `codex-gpt5.6-sol` 2026-07-17** — full runbook in `dhnt/docs/steward-handover-2026-07-17.md` (the steward loop, commit/pin/refresh workflow, room control surface, disciplines, watches). Claude is observer/assistant.
**Last updated**: 2026-06-18 (array2 FLIPPED via the quoted-`@`-vs-IFS fix in sh/expand — `"${a[@]}"`/`"$@"` split to one word per element regardless of IFS; also dropped dollars 141→102 + exp-tests 61→52. glob-test 88→85 (bash-correct trailing-`\` literal + `?` leading-dot in sh/pattern, not yet a flip). Earlier: array/assoc/nameref/new-exp/coproc flipped; harness now measures the 8 formerly-silent skips — `<name>.tests` mapping mismatch — so the scoreboard finally covers every fixture instead of hiding 8):
  - Wired into the harness (name→file mappings, like `dirstack`→`dstack`): array2→array-at-star, dollars→dollar-at-star, exp-tests→exp.tests(+expect-filter), glob-test→glob.tests, histexpand→histexp.tests, input-test→`< input-line.sh`.
  - `run-minimal` excluded (a `run-all`-style meta-runner, no stable `.right`). `execscript` skipped with a reason (host-dependent: bash binary path + system error wording + exec/`.`-on-directory exit codes; needs `test`-style normalization to measure).
  Reliable scoreboard = `make test-bash` under a clean PATH (`PATH=/bin:/usr/bin:$(dirname $(which go))`; a shell wrapper in PATH shadows `sh` and false-fails). weave sandboxes need the external/bash-5.3 fixture symlink prepped (it's a gitignored symlink) or workers can't measure and gates false-pass.

**Remaining failing fixtures: NONE.** (2026-06-18: dollars + exp-tests FLIPPED [claude]; glob-test FLIPPED [claude] via byte-transparency — bashy now follows GNU bash 5.3's LC_CTYPE/MB_CUR_MAX convention exactly (no UTF-8 hardcoding): `$'\u'` encodes in the locale charset (= u32cconv), the lexer treats invalid/incomplete multibyte as opaque single bytes (= MB_INVALIDCH→1, never errors), and read/IFS split per MB_CUR_MAX — so the zh_TW.big5 case matches bash. NOT a ceiling after all.)
**trap FLIPPED 2026-06-18** [claude] — startup-ignored signals can't be re-trapped (`trap.c`: real SIG_IGN + `unix.Sigaction` snapshot); SIGCHLD trap fires once per reaped child (`jobs.c:waitchld`).
**execscript FLIPPED 2026-06-18** [claude WIP salvaged + codex finish, cross-repo] — exec/run exit codes 126 (EX_NOEXEC) vs 127 (EX_NOTFOUND), `command_not_found_handle` hook, exec/`.`-on-directory wording (sh interp); `bash -i`→`expand_aliases=on` + EOF EXIT-trap cleanup (bashy main.go); EXIT-trap-in-subshell + BASH_SUBSHELL nesting. NOT host-locked after all. (Also added the execscript→exec.right harness mapping that had silently skipped it.)
**jobs FLIPPED 2026-06-19** [claude Phase-1 salvaged + codex Phase-2, the final lap] — real process-group job control on unix: `setpgid` backgrounded external jobs, `Wait4(WUNTRACED|WCONTINUED)` stopped-state tracking, `kill -STOP/-CONT` + `fg`/`bg` resume, jobs-list `Stopped` rendering, `suspend` refusal messages. All OS code in sh's `*_unix.go` — mirrors bash's own `jobs.c` (unix) / `nojobs.c` (elsewhere) split, so unix-gated is bash-faithful, not a compromise. Phase 1 (builtin logic: wait-on-done-pid, fg/bg/disown jobspec + options, rendering) caught + fixed an `assoc` regression via the full-suite gate. Needs `BASH_TEST_TIMEOUT_JOBS`. NOT a ceiling.
**Skipped: NONE.** Every bash-5.3 fixture passes.

---

## Completed Phases

- [x] **Phase 1**: Foundation — cmd/bashy, prompt expansion, version vars
- [x] **Phase 2**: Parameter expansion @U/@u/@L/@K/@k/@P, pe.Width, pe.IsSet
- [x] **Phase 3**: Trap system (EXIT/ERR/DEBUG/RETURN), signal names, trap -l/-p
- [x] **Phase 4a**: caller, hash, help, enable builtins, call stack
- [x] **Phase 4b**: history/fc/bind stubs
- [x] **Phase 4c**: Job control stubs (jobs/fg/bg/kill/disown/wait -n/-p)
- [x] **Phase 5**: Shopt options (nocasematch, xpg_echo, autocd, inherit_errexit, sourcepath)
- [x] **Phase 6**: Programmable completion stubs (compgen/complete/compopt)
- [x] **Phase 7**: Coproc execution, BASH_REMATCH
- [x] **Phase 8**: FUNCNAME/BASH_SOURCE/BASH_LINENO call stack arrays
- [x] **Phase 9**: Readline via ergochat/readline (MIT, pure Go)
- [x] **Phase 10**: Persistent history (~/.bashy_history) — basic via readline
- [x] **Phase 11**: Startup files (.bashrc, .bash_profile, /etc/profile, BASH_ENV)
- [x] **Phase 12**: Named FD redirections ({varname}> basic detection)
- [x] **Phase 13**: Shell variables (HOSTNAME, HOSTTYPE, OSTYPE, SHELLOPTS, BASHOPTS, SHLVL, PIPESTATUS, BASH_ARGV0, GROUPS)
- [x] **Phase 14**: read -t/-n/-d/-e/-i/-u options
- [x] **Phase 20**: Test harness (make test-bash with 15s per-test timeout)
- [x] **Parser**: ${var~}/${var~~} case-toggle operators
- [x] **Parser**: Pattern panic fix (regexp.Compile instead of MustCompile)
- [x] **Interp**: noclobber (-C) and posix options
- [x] **Interp**: declare -F, declare -i
- [x] **Interp**: export/readonly/local/declare as builtin commands (not just keywords)
- [x] **Interp**: echo combined flags (-en, -neE)
- [x] **Expand**: printf #/' flags, . precision, float formats, uppercase X
- [x] **Interp**: Positional params >9 (${10}, ${11}, etc.)

---

## Remaining Work — By Priority

### P0: Parser Fixes (blocking entire test files)

- [x] `+=` compound assignment in arithmetic ternary: `$((cond ? val : x+=2))`
- [x] Empty heredoc delimiter: `cat <<''` (already worked; regression tests added)
- [x] `${ cmd; }` funsub (brace command substitution) execution — body runs in caller (no fork), stdout captured; bash 5.3 scope semantic (all assignments local to body) via funsubScope on overlayEnviron
- [x] `${ (shift) }` funsub with subshell — already worked once funsubScope landed (subshell isolates positional-param changes; shift in a bare funsub correctly leaks per bash 5.3); regression tests pin all three shapes
- [x] `${H*}` — `*` as parameter expansion pattern inside `[[ ]]` — root cause was eager rhs evaluation in `[[ a && b ]]` / `[[ a || b ]]`; short-circuit so unevaluatable expansions on rhs never run when lhs settles result
- [x] `((true ) )` — arithmetic with space before `)` in case clause (peekArithmEnd skips horizontal whitespace)
- [x] `case esac in esac)` — verified: bash 5.3 *rejects* bare `esac)` as a pattern (POSIX grammar rule 4: "when PATTERN == ESAC, return ESAC"). Only `(esac)` parenthesized or `foo|esac)` post-pipe forms are accepted, and bashy already handles both. Earlier attempt (ee9202fb) accepted the bare form too, which regressed `parser.tests`; reverted.

### P1: Error Message Format (affects ~60 tests)

- [x] Add `<filename>: line <N>:` prefix to error messages from builtins — added `r.bashErrPrefix(pos)` to the primary-error errf calls in `builtin()` (kill, alias, enable, help, dirs/pushd/popd, printf, getopts, trap) and the declare/local/local-only paths in runner.go
- [x] Add `<filename>: line <N>:` prefix to error messages from setVar/readonly — setVar already had it; delVar (the unset path) was missing — now uses the same `r.bashErrPrefix(r.curStmtPos)` pattern
- [x] Match bash error message wording exactly (e.g., `readonly variable` → same) — fixed: unset of invalid identifier (`unset 1bad` → "not a valid identifier", exit 2); shift wording (`abc` → numeric argument required, `-1` → shift count out of range, extras → too many arguments, exit 1); other wording covered by items 55-57
- [x] Error messages for `printf` should match bash format — added `cfg.OnFormatWarning` callback for soft conversion failures (`printf %d xyz` → "invalid number" warning + exit 1 + continue with 0). Hard errors (invalid format char, missing format char, not a valid identifier for -v) already matched.
- [x] Error messages for `read` should match bash format — fixed `read 1bad` and `mapfile 1bad` wording to ``read: `1bad': not a valid identifier`` (was `invalid identifier "1bad"`). All other read errors already matched.
- [x] Use backtick quoting style matching bash (`` ` `` vs `'`) — converted `%q` to bare option form for `declare -X` / `trap -X` / `pwd -X` (bash uses bare for options); identifier-bearing errors already use ``%q': not a valid identifier`` form per the prior commits.

### P2: Builtin Enhancements (affects ~30 tests)

- [x] `printf -v var` — write output to variable instead of stdout
- [x] `printf %b` — interpret backslash escapes in argument (already worked; regression tests added)
- [x] `printf %(fmt)T` — datetime formatting (strftime subset; -1 = now, -2 = shell start, integer = Unix timestamp)
- [x] `printf --` — argument terminator (already worked via flagParser; regression test added)
- [x] `printf` full error handling matching bash — extended OnFormatWarning to the float (`%f/%e/%g/%G/%E`) path; switched the remaining `%q` quoting to bare in the width-arg + `%(fmt)T` time-arg invalid-number paths so the wording is bash-identical.
- [x] `declare -f` display format matching bash (indentation, semicolons) — printFuncDecl now re-indents every printer-output line by 4 spaces (so nested blocks land at 8/12/...) and appends `;` to each simple statement (with heuristic skip for openers/closers and the last top-level stmt). Output of `declare -f` on `foo() { if [ 1 ]; then a=1; b=2; fi; }` now matches bash 5.3 exactly.
- [ ] `declare -p` output format matching bash
- [ ] `declare -i` integer arithmetic on assignment
- [x] `type -t` — output just type name (alias/keyword/function/builtin/file)
- [x] `type -a` — show all matches (factored through typeMatches helper)
- [x] `type -f` — skip function lookup
- [x] `type -p` — print path only if no higher-priority match
- [x] `type -P` — force PATH search
- [x] `command -V` — verbose command description (reuses typeMatches)
- [x] `return` outside function — already errors with proper message
- [x] `let` with multiple expressions — already worked; regression tests added
- [x] `select` loop construct — rewrote to actually loop and handle EOF/empty/invalid
- [x] `mapfile -O origin` (pad lower indices), `-s skip`, `-n max`, `-c quantum`, `-C callback` (callback receives `idx quoted-line`)
- [x] `read -N` nchars (don't stop at delimiter; assigns the raw buffer to the first variable, no IFS split). `-n` now reads byte-by-byte so it stops correctly at the delimiter.
- [x] `getopts` OPTERR variable (OPTERR=0 silences diagnostics regardless of leading `:` in optstring); error-message format still pending

### P3: Expansion/Quoting Fixes (affects ~20 tests)

- [ ] Brace expansion with backslash quoting: `\{a,b\}` should not expand
- [x] Brace expansion sequence step: `{0..10..2}` step handling (now uses |step| with sign matching range direction; {10..1..2} → 10 8 6 4 2)
- [x] Brace expansion zero-padding: `{01..05}` → 01 02 03 04 05 (now also handles mixed widths like `{01..100}` and negative ranges)
- [x] `$'...'` ANSI-C `\cX` control-char escape (\cA → 0x01, \c@ → 0x00 etc.) — other ANSI-C escapes already worked
- [ ] IFS scoping: temporary IFS in simple commands vs eval/special builtins
- [ ] Word splitting with empty fields (IFS-related)
- [x] Tilde expansion in assignments: `PATH=~:$PATH` (LiteralForAssign + tildeInAssign flag)
- [ ] `$"..."` locale translation strings
- [x] Arithmetic base notation: `16#FF`, `2#1010` (bases 2-64 with bash's extended digit alphabet for 37-64)

### P4: Shell Variable Completeness

- [x] `BASH_COMMAND` — set dynamically before each command (pre-expansion via printer for CallExpr, post-expansion for builtin/exec)
- [x] `BASH_EXECUTION_STRING` — set by cmd/bashy from the -c argument (env-passed before runner construction)
- [x] `BASH_SUBSHELL` — verified: 0 at top, increments per nested `( ... )` subshell
- [x] `COLUMNS` / `LINES` — terminal dimensions via term.GetSize() (probes stdin/stdout/stderr; empty when no TTY)
- [x] `PROMPT_DIRTRIM` — truncate \w in prompts (positive integer keeps last N components, prepends ".../")
- [x] `HISTCMD` — current history number (set per interactive command, incrementing)
- [ ] `COMP_*` variables (COMP_WORDS, COMP_CWORD, COMP_LINE, COMP_POINT, COMPREPLY)
- [x] `BASH_ALIASES` — associative array of aliases (dynamic from r.alias, reprinted via syntax.Printer)
- [x] `BASH_CMDS` — associative array of hash table (dynamic from r.cmdHashTable)
- [ ] `BASH_COMPAT` — compatibility level (settable/readable as a regular variable; we always behave as bash 5.3 so the value has no effect)
- [ ] `BASH_XTRACEFD` — redirect xtrace to FD
- [x] `MAIL` / `MAILCHECK` / `MAILPATH` — settable/readable as plain variables; no periodic mail check loop (intentionally — modern shells skip this)
- [ ] `READLINE_LINE` / `READLINE_POINT`

### P5: Interactive Features

- [ ] History expansion: `!!`, `!$`, `!n`, `!-n`, `!string`, `^old^new`
- [ ] `history` builtin: -c (clear), -d (delete), -a (append), -r (read), -w (write)
- [ ] `fc` builtin: -l (list), -s (re-execute), -e (edit)
- [ ] `bind` builtin: -p (list), -x (key to command)
- [ ] Programmable completion: compgen/complete/compopt full implementation
- [ ] Tab completion wired to readline
- [ ] `PROMPT_COMMAND` execution (done basic, needs array support)
- [ ] `PS0` display after command read, before execution
- [ ] `PS4` custom xtrace prefix (replace hardcoded "+ ")
- [ ] SIGWINCH → update COLUMNS/LINES

### P6: Job Control (real process groups)

- [ ] Process group management (Setpgid in exec.Cmd.SysProcAttr)
- [ ] Terminal control (tcsetpgrp)
- [ ] SIGTSTP (Ctrl-Z) to stop foreground job
- [ ] `fg %n` — tcsetpgrp + SIGCONT + wait
- [ ] `bg %n` — SIGCONT without terminal control
- [ ] `jobs` — proper status display (running/stopped/done)
- [ ] `kill` — send signals to process groups
- [ ] `disown -h` — mark jobs to not receive SIGHUP
- [ ] `wait -f` — wait for job to terminate (not just change state)

### P7: Remaining Shopt Options

- [ ] `checkjobs` — warn about running/stopped jobs on exit
- [ ] `cdspell` / `dirspell` — spelling correction
- [ ] `histappend` — append to history file on exit
- [ ] `histreedit` / `histverify` — re-edit/verify history substitutions
- [ ] `cmdhist` / `lithist` — multi-line history formatting
- [ ] `execfail` — don't exit on exec failure
- [ ] `localvar_inherit` / `localvar_unset` — local variable scoping
- [ ] `extdebug` — extended debugging
- [ ] `compat31` through `compat44` — version compatibility modes
- [ ] `direxpand` — expand directory names in completion
- [ ] `globasciiranges` — wire to pattern matching (marked supported, verify)
- [ ] `progcomp` / `progcomp_alias` — programmable completion

### P8: Polish

- [ ] `help` builtin with proper embedded text per builtin (//go:embed)
- [ ] `times` with real rusage data (syscall.Getrusage)
- [ ] Named FD redirections: allocate real FD numbers, close support
- [ ] `exec` replacing the process (unix.Exec)
- [ ] `.` (source) line number tracking for error messages
- [ ] Function display format matching bash exactly
- [ ] Heredoc with tabs (<<-) indentation stripping edge cases

### P9: POSIX Compliance

- [x] Obtain Open Group VSC-PCTS test suite license; licensed materials remain
      outside git and utilities results remain withheld pending scope follow-up
- [ ] Create tests/posix/ with POSIX shell compliance tests
- [ ] ShellSpec integration for portability testing
- [ ] POSIX mode (set -o posix) behavioral differences

---

## Test Progress Tracking

Snapshot from `make test-bash` on 2026-06-09: **63 PASS, 13 FAIL, 11 SKIP**
(diff line counts are `diff <bashy-output> <name>.right | wc -l`, lower = closer to passing).

### Passing (63)

```
alias         appendop      arith-for     attr          braces
case          casemod       complete      comsub        comsub-eof
comsub-posix  comsub2       cond          cprint        dbg-support2
dirstack      dynvar        exportfunc    extglob       extglob2
extglob3      func          getopts       glob-bracket  globstar
herestr       ifs           ifs-posix     intl          invert
invocation    iquote        lastpipe      mapfile       more-exp
nquote        nquote1       nquote2       nquote3       nquote4
nquote5       parser        posix2        posixexp2     posixpat
posixpipe     precedence    printf        procsub       quote
read          redir         rhs-exp       rsh           set-e
set-x         shopt         strip         test          tilde
tilde2        type          vredir
```

### Failing (13, sorted by previous diff size)

| Test | Diff Lines | Likely blocker |
|------|-----------:|----------------|
| quotearray   | 155 | Array quoting |
| heredoc      | 171 | Heredoc edge cases |
| posixexp     | 311 | POSIX expansion |
| varenv       | 366 | Variable/environment |
| arith        | 372 | Arithmetic edge cases |
| dbg-support  | 377 | DEBUG trap / source-line tracking |
| history      | 399 | `history` builtin (G2, G11) |
| assoc        | 412 | Associative array edge cases |
| builtins     | 509 | Misc builtins |
| nameref      | 591 | Name references |
| new-exp      | 813 | New expansion features |
| array        | 855 | Indexed array edge cases |

### Skipped (11)

- `coproc`, `jobs`, `trap` — skipped via `BASH_TEST_SKIP` in Makefile (need controlling TTY / real job control)
- 8 silent skips — bash test-suite runners with no matching `.tests` or `.right` file in the vendored tree

---

## Quick Reference

```bash
# Run all Go tests
go test ./...

# Run bash 5.3 test suite
make test-bash

# Run single bash test
cd external/bash-5.3/tests
THIS_SH=../../../bin/bashy PATH=$PWD:$PATH ../../../bin/bashy ./<name>.tests

# Compare output
diff <output> <name>.right
```

---

## Bash 5.3 Gap Analysis (from comprehensive audit, 2026-05-26)

Full reports in `docs/bash-gap-analysis.md` and `docs/agentic-extensions.md`.
Items below are organized by priority and tagged by effort: S (1 commit),
M (a session), L (multi-session), XL (cross-cutting). Anything already
covered by an earlier section above is NOT repeated here.

### G0: Error-format pass (M, unlocks ~60 bash 5.3 tests)

- [ ] `<file>: line N:` prefix on every `failf` site (use `r.curStmt` pos)
- [ ] `<name>: usage: ...` ordering (vs. current `usage: <name>`) — match `printf`, `read`, `getopts`, etc.
- [ ] Quote style: bash uses `` `foo' `` (backtick + single-quote); bashy uses `'foo'` — change globally
- [ ] Exact wording match for: `command not found`, `bad substitution`, `not a valid identifier`, `readonly variable`, `unbound variable`, `cannot create temp file`, `arithmetic syntax error`
- [ ] Verify `bash --posix` mode output matches bash's `--posix` variants

### G1: Parser blockers (XL, unlocks 6 tests)

- [x] `${ cmd; }` funsub parser production (`syntax/parser.go:1247`, `CmdSubst.TempFile`), runtime runs body in caller's scope (`interp/runner.go:91`) — shipped under P0
- [x] `${ (shift) }` subshell-within-funsub — shipped under P0 (subshell isolates positional-param changes)
- [x] `${H*}` — short-circuit unevaluatable rhs in `[[ a && b ]]` / `[[ a || b ]]` so `${H*}` never runs when lhs settles result — shipped under P0
- [x] `((true ) )` — accept whitespace before closing `)` in case-clause arithm (`peekArithmEnd` skips horizontal whitespace) — shipped under P0
- [x] `case esac in esac)` — N/A: bash 5.3 rejects bare `esac)` per POSIX rule 4. `(esac)` and `foo|esac)` work in bashy.
- [x] `${|cmd;}` valsub — `CmdSubst.ReplyVar` parsed at `syntax/parser.go:1250`, runtime captures body's `REPLY` as expansion value at `interp/runner.go:105-124`

### G2: Stub builtins worth finishing (M each)

- [ ] `complete`/`compgen`/`compopt` — full spec engine (`-F/-W/-G/-C/-A/-X/-P/-S/-o`), wire to readline tab callback (L)
- [ ] `history` — `-c/-d/-a/-r/-w/-n/-s/-p` on `~/.bashy_history` (M)
- [ ] `fc` — `-l/-s/-e/-n/-r` re-execute and edit (M)
- [ ] `bind` — `-p/-l/-x KEYSEQ:command/-r/-q/-u/-m keymap/-f file` (M)
- [ ] `disown -h` — mark jobs to skip SIGHUP (S)
- [ ] `help` — embed bash-style per-builtin help text (//go:embed) (S)
- [ ] `times` — `syscall.Getrusage(RUSAGE_SELF/CHILDREN)` (S)
- [ ] `ulimit` — at minimum: `-n` (file desc), `-u` (procs), `-t` (cpu time), `-f` (file size); respect cap from `setrlimit` (M)

### G3: Builtin completeness (S–M each)

- [x] `mapfile -O origin`, `-c count`, `-C callback`, `-s count`, `-n max`, `-u fd` (fd accepted but reads stdin)
- [x] `read -N nchars` (distinct from `-n`): exact-count, no delimiter handling, no IFS split
- [ ] `read -a array` for assoc arrays
- [ ] `declare -p` formatting matching `subst.c:string_var_assignment`
- [ ] `declare -f NAME` formatting matching bash (indent, semicolons, function header)
- [ ] `declare -i` enforce arithmetic-on-assignment for subsequent assignments
- [ ] `declare -u/-l/-c` case-attribute auto-transform (`att_uppercase`/`lowercase`/`capcase`)
- [ ] `printf %q` to use bash's `sh_quote_reusable` style
- [x] `kill -L` (uppercase = signal table) — accepted as an alias for `-l` in the kill builtin
- [ ] `getopts` OPTERR variable, leading-colon-in-optstring silent mode
- [ ] `caller -e EXTDEBUG` extended-debug semantics
- [ ] `command --explain foo` (new; from agentic extensions)

### G4: Variables — secondary set (S each)

- [x] `BASH_COMMAND` set before *every* simple command, not just traps
- [x] `BASH_EXECUTION_STRING` — store `-c` argument
- [ ] `BASH_COMPAT` — accept and validate compatibility level
- [ ] `BASH_XTRACEFD` — redirect xtrace output to FD
- [x] `BASH_ALIASES` — dynamic assoc array of aliases
- [x] `BASH_CMDS` — dynamic assoc array of hashed paths
- [ ] `BASH_ARGV`/`BASH_ARGC` — function-call argv stack (requires `extdebug`)
- [x] `BASH_MONOSECONDS` — monotonic clock (new in 5.3) — uses time.Since(startTime) which keeps Go's monotonic component
- [x] `HISTCMD` — current history entry number
- [ ] `HISTCONTROL`, `HISTIGNORE`, `HISTTIMEFORMAT` — history filtering
- [x] `FUNCNEST` — function recursion limit (positive integer aborts call when callStack depth reached; 0/unset/empty/non-numeric disables)
- [ ] `EXECIGNORE` — skip-exec patterns for command lookup
- [ ] `GLOBIGNORE` — glob-skip patterns
- [x] `IGNOREEOF` — Ctrl-D count before exit (positive int = N additional EOFs, non-numeric = bash's default of 10, unset/empty = exit on first EOF)
- [ ] `INPUTRC` — readline init file path
- [x] `OPTERR` — getopts error-print flag (OPTERR=0 suppresses messages; covered with the getopts diagnostics path)
- [ ] `PROMPT_COMMAND` as array — iterate all entries
- [x] `PROMPT_DIRTRIM` — truncate `\w`
- [ ] `PS0` — print after read, before exec
- [ ] `PS4` — replace hardcoded `+ ` in trace.go with expanded PS4
- [ ] `TIMEFORMAT` — for `time` builtin output
- [ ] `TMOUT` — interactive idle / `read` default timeout
- [x] `LINES`, `COLUMNS` — terminal dimensions via `golang.org/x/term`
- [x] `OLDPWD` — set by cd to the previous PWD; `cd -` chdirs back and echoes it
- [ ] `COMP_WORDS`, `COMP_CWORD`, `COMP_LINE`, `COMP_POINT`, `COMP_KEY`, `COMP_TYPE`, `COMPREPLY`, `COMP_WORDBREAKS` — set during completion functions
- [ ] `READLINE_LINE`, `READLINE_POINT`, `READLINE_MARK` — set during `bind -x` callbacks

### G5: Variable attributes (M)

- [ ] `declare -u` / `att_uppercase` — auto-uppercase on assignment
- [ ] `declare -l` / `att_lowercase` — auto-lowercase on assignment
- [ ] `declare -c` / `att_capcase` — auto-capitalize on assignment
- [ ] `att_invisible` — variable exists but has no value yet
- [ ] `att_trace` — function tracing for `set -o functrace`

### G6: `set -o` options (S each)

- [ ] `braceexpand` `-B` — accept toggle (always on)
- [ ] `emacs` / `vi` — switch readline edit mode
- [ ] `errtrace` `-E` — ERR trap inheritance
- [ ] `functrace` `-T` — DEBUG/RETURN trap inheritance
- [ ] `hashall` `-h` — toggle command hashing
- [ ] `ignoreeof` — Ctrl-D count before exit
- [ ] `interactive-comments` — `#` in interactive shells
- [ ] `keyword` `-k` — all `name=value` treated as env
- [ ] `notify` `-b` — async notify of bg completion
- [ ] `onecmd` `-t` — exit after one command
- [ ] `physical` `-P` — don't resolve symlinks in cd
- [ ] `privileged` `-p` — disable startup files and `$ENV`

### G7: Shopt options (S each)

- [ ] `globskipdots` — skip `.`/`..` in `*` (new in 5.3, default on)
- [ ] `patsub_replacement` — `&` in replacement of `${var//pat/rep}` (default on in 5.3)
- [ ] `noexpand_translation` — suppress `$"..."` translation
- [ ] `varredir_close` — close named-fd on stmt exit
- [ ] `bash_source_fullpath` — full path in BASH_SOURCE (new in 5.3)
- [ ] `array_expand_once` — controls re-expansion in `[[ ]]`
- [ ] `extdebug` — enable BASH_ARGV/BASH_ARGC stack, `caller`-with-source line
- [ ] `localvar_inherit` — local vars inherit value from enclosing scope
- [ ] `localvar_unset` — local vars without value start unset (not "")
- [ ] `cdspell`, `dirspell` — Levenshtein corrections
- [ ] `restricted_shell` — actually enforce restrictions (for `rsh` test)
- [ ] `histappend`, `histreedit`, `histverify`, `cmdhist`, `lithist`, `mailwarn` — connect to history backend
- [ ] `login_shell` — reflect `WithLoginShell` state in `shopt -p`

### G8: Job control phase 1 (L)

- [ ] `Setpgid: true` on `exec.Cmd.SysProcAttr` (Unix)
- [ ] Track per-bgProc `pgid`
- [ ] `kill %N` resolves to pgid; signals whole group
- [ ] `kill 0` — signal current process group
- [ ] Jobspec parsing: `%+`, `%-`, `%?str`, `%str`, `%%`
- [ ] `jobs -p` (PID only), `-l` (long format with PID), `-n` (changed-since-last), `-r` (running), `-s` (stopped), `-x cmd` (substitute jobspec)
- [ ] `[1]+ Done <cmd>` status notification on prompt

### G9: Job control phase 2 (XL)

- [ ] TTY control (`tcsetpgrp` via golang.org/x/sys/unix)
- [ ] SIGTSTP (Ctrl-Z) handler — stop foreground job, push to bg table
- [ ] `fg %N` — tcsetpgrp + SIGCONT + wait, restore TTY on exit
- [ ] `bg %N` — SIGCONT only
- [ ] `wait -f` — wait for terminal state, not just status change

### G10: Readline depth (L)

- [ ] Tab completion through `complete`/`compgen` registry (depends on G2)
- [ ] `bind -p` / `-l` / `-x KEYSEQ:cmd` / `-r` / `-q` / `-u` / `-f file`
- [ ] `~/.inputrc` / `/etc/inputrc` parsing (consider `xo/inputrc`)
- [ ] `set -o vi` / `set -o emacs` mode switching at runtime
- [ ] SIGWINCH handler — update `COLUMNS`/`LINES`

### G11: History expansion (M, separate from `history` builtin)

- [ ] `!!`, `!N`, `!-N`, `!str`, `!?str?`, `!$`, `!*`, `!:N`, `!:N-M`
- [ ] `^old^new^` substitution
- [ ] Modifiers: `:h`, `:t`, `:r`, `:e`, `:p`, `:s/old/new/`, `:&`, `:g`, `:a`
- [ ] `histchars` variable (default `!^#`) — change the trigger char

### G12: Locale and i18n (M)

- [ ] `$"..."` gettext translation (use `golang.org/x/text/message`)
- [ ] `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_NUMERIC`, `LC_TIME` — wire through `unicode/utf8` and `time` formatters
- [ ] Case modification respect locale (currently uppercase via Unicode tables only)

### G13: Agentic extensions (see docs/agentic-extensions.md)

- [x] **#1 Deterministic mode**: `set -o deterministic`, `BASHY_DETERMINISTIC=N` (S–M)
- [ ] **#2 `--json` flag** on `jobs`, `declare -p`, `declare -F`, `trap -p`, `set`, `set -o`, `shopt -p`, `type`, `times`, `kill -l` (S each, do all in one session)
- [x] **#3 `runner-state` builtin** with subcommands `vars`/`traps`/`fds`/`opts`/`callstack`/`all` (S)
- [ ] **#4 Resource limits**: `WithMaxWallTime`, `WithMaxCPUTime`, `WithMaxOutputBytes`, `WithMaxChildProcs`, `WithMaxOpenFiles`; new builtin `limits` (M)
- [ ] **#5 Sandbox mode**: `WithSandboxRoots(read, write)`, `BASHY_SANDBOX_READ/WRITE` env, `sandbox-status` builtin (M)
- [x] **#6 Audit hook**: `WithAuditHandler(func(AuditEvent))`, optional `BASHY_AUDIT_LOG=path.jsonl` (S)
- [ ] **#7 Dry-run mode**: `--dry-run` flag emitting `[would-run]` per leaf cmd; `command --explain foo` (M for full, S for explain only)
- [ ] **#8 Capability declarations**: `# bashy: requires net,fs-write` preamble + `require` builtin + `WithCapabilities(set)` option (S–M)
- [x] **#9 Structured errors**: `WithStructuredErrors(func(ErrorEvent))` carrying kind/severity/pos/function (S)
- [ ] **#10 Record / replay**: `BASHY_RECORD=path.jsonl` and `bashy --replay file [--strict|--lax]` (M)
- [ ] **#11 Inline docs**: `bashy explain <name>` from `//go:embed help/*.md` (S; value is content)
- [ ] **#12 Cancellation audit**: verify `ctx.Done()` propagates into all loops/bg procs; add `WithCancelHook` (M)
- [ ] **#13 Embedder builtins**: `WithExtraBuiltins(map[string]BuiltinFunc)` (S)
- [ ] **#14 Metrics handler**: `WithMetricsHandler(func(Metric))` (S)
- [ ] **#15 Policy file**: `~/.bashy/policy.toml` or `.bashy.toml` with options/deny/caps sections (M)

### Recommended next batches (from gap-analysis Section "Recommended next batches")

1. **Batch A**: Error-message format pass (G0) — ~60 tests for one session
2. **Batch B**: `${ cmd; }` funsub + parser fixes (G1) — XL, unlocks comsub/comsub2/cond/parser tests
3. **Batch C**: Agentic batch 1 — G13 items #1 (deterministic), #6 (audit), #2 (json), #3 (runner-state)
4. **Batch D**: Job control phase 1 (G8)
5. **Batch E**: Programmable completion (part of G2)
