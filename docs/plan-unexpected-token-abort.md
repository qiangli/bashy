# Plan: abort on malformed case / misplaced-keyword parse errors like bash

Status: SHIPPED (case + reserved-word abort); command-sub__028 DEFERRED.

## Problem

Drop-in fidelity probe (`scripts/bash-fidelity.sh`, oils corpus) surfaced three
diffs where our pure-Go `bash` (cmd/bash) diverged from GNU Bash 5.3 on parse
errors. bash treats these as a *fatal* "syntax error near unexpected token
`X'" and aborts the whole input (status 2), running nothing on the offending
line. Our generic statement-by-statement recovery instead ran the line's prefix
as a command and continued.

```
case___012.sh   `case\nin esac`
   bash : ./s: line 1: syntax error near unexpected token `newline'
          ./s: line 1: `case'
   ours : ./s: line 1: syntax error near unexpected token `case'   (wrong token)
          ./s: line 1: `case'
          ./s: line 2: in: command not found                       (should not run)

command-sub__007.sh   `$(echo f)$(echo or) i in a b c; do echo $i; done`
   bash : ./s: line 1: syntax error near unexpected token `do'
          ./s: line 1: `$(echo f)$(echo or) i in a b c; do echo $i; done'
   ours : ./s: line 1: for: command not found                      (ran the prefix)
          ./s: line 1: `do` can only be used in a loop              (wrong wording)
          ./s: line 1: `$(echo f)$(echo or) i in a b c; do echo $i; done'
```

## Fix (shipped)

Generalize the existing up-front `[[ ]]` abort. `internal/cli/main.go`'s
`run()` already does a whole-input pre-parse and, on error, calls
`syntax.DbracketParseError` to abort with bash's wording for malformed
conditionals. Added a sibling CLI helper invoked right after it:

- `unexpectedTokenAbort(err, src, prefix) (msg, ok)` — renders bash's two-line
  `syntax error near unexpected token` diagnostic and returns ok so `run()`
  aborts with `ExitStatus(2)`. Returns ok=false for any other ParseError, so
  the recovery path is unchanged (same contract as `DbracketParseError`).
- `unexpectedTokenName` recognizes two families and names bash's token:
  - **case with no subject word** (`` `case` must be followed by a word `` /
    `` `case x` must be followed by `in` ``) — bash names the token it found
    instead of the subject; a bare `case<newline>` is reported as `newline`
    (reuses the existing `tokensToSkip`/`offendingTokenAfter` helpers; empty →
    `newline`).
  - **loop/`if`/`case` reserved word out of place** (`do`, `done`, `then`,
    `elif`, `fi`, `esac`, `;;`) via `reservedWordOnlyError` — bash names the
    reserved word itself.
- Two guards keep it conservative:
  - **Open command substitution** — inside an unclosed `$(`, bash reports the
    matching-`)' variant, so `commandSubstOpenBefore` declines those (and
    `done`/`esac`/`;;` are additionally declined whenever the source merely
    contains a `$(`, since the recovery path already rewrites those to the
    matching-`)' wording — avoids clashing with established behavior).
  - **Prior-line statement** — bash reads-parses-executes one newline-terminated
    command at a time, so a complete statement on an *earlier* line runs before
    it reaches the offending one. `completeStmtBeforeLine` declines the up-front
    abort when such a statement exists, so we never skip output bash would have
    produced. (Clauses on the *same* line as the error — like `007`'s
    `$(...) i in a b c;` before `do` — are part of the failing parse unit and
    never run, matching bash. The two corpus cases both error on line 1.)

Tests: `TestRunUnexpectedTokenAbort` (case-missing-subject, keyword-out-of-loop,
prior-line-statement-runs-first) in `internal/cli/main_test.go`.

## command-sub__028 — DEFERRED (not an abort)

```
$SH -c 'echo `echo "`'        # unclosed " inside a backtick command sub
   bash : bash: command substitution: line 1: unexpected EOF while looking for matching `"'
          <blank line from the outer echo>
          status=0
   ours : bash: -c: line 1: reached "`" without closing quote `"`
          status=2
```

This is the *opposite* of an abort: bash treats a malformed backtick command
substitution as non-fatal — it prints a `command substitution: line N:` warning,
substitutes empty, runs the *outer* command, and exits 0. Reproducing it needs
interpreter-level command-substitution parse-error recovery in
`mvdan.cc/sh/v3` (`interp`/`syntax`), not a CLI-level abort, with real
regression risk to every `$(...)`/backtick path. Out of scope for this
abort-focused round; tracked as a follow-up.

## Gates

- `go test ./...` — green except the pre-existing
  `TestRunNestedBadSubstRecoveryContinues` (fails on baseline too; a parallel
  `../sh` round, per CLAUDE.md "conflicts with parallel sh rounds are fine").
- `make test-bash` must stay 86/86 — cannot run locally (no `external/bash-5.3`
  fixture symlink in this checkout); the orchestrator runs it post-merge. The
  abort only fires when the whole-input pre-parse fails, so valid scripts are
  untouched, and the prior-line guard prevents skipping earlier output.
