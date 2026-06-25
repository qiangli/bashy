# Array parse-error fidelity (issue-9)

Status: SHIPPED (2 real fixes); 2 cases reclassified as harness artifacts.

## Source

`scripts/bash-fidelity.sh` (oils corpus, each case run as `./s`, ours vs real
`bash:5.3`) surfaced 4 diffs in the array / array-assign / assign-extended
suites. All 4 were re-verified against `docker.io/library/bash:5.3` in podman
before acting. Two are genuine `cmd/bash` parse-error rendering bugs; two are
NOT shell bugs — they are `$SH`-guard harness artifacts (the task brief
misdiagnosed these as "extend `unexpectedTokenAbort`" cases).

## Fix 1 — array-assign__006: unterminated array subscript (`a[`)

```
$SH -c 'a['
```

Our parser reports `` `[` must be followed by an expression `` (an *incomplete*
parse — EOF reached while expecting the subscript expression). bash reports the
matching-`]'` EOF wording and omits the source-line echo:

```
bash: -c: line 1: unexpected EOF while looking for matching `]'
```

`internal/cli/main.go` `rewriteParserErrorText`: when `pe.Incomplete &&
pe.Text == "`[` must be followed by an expression"`, return `unexpected EOF
while looking for matching `]'`. `printBashParseError` already suppresses the
source-line echo for any `unexpected EOF …` text. The sibling subscripts in the
same fixture (`a[5`, `a[5 + `) already rendered correctly; only the bare-`[`
incomplete case was wrong.

## Fix 2 — array__005: stray `)` aborts (wording + recover-vs-abort)

```
a=(
1
&
'2 3'
)
argv.py "${a[@]}"
```

bash recovers from the mid-array `&` (a control operator) — reporting it and
running the following `'2 3'` as a command (status 127) — but the stray `)` on
line 5 is a *fatal* parse error: bash names it as a generic unexpected token and
**aborts the whole input** (status 2), so the trailing `argv.py` line never runs.

Our parser reports `` `)` can only be used to close a subshell `` and our
generic statement recovery ran the trailing line (emitting argv.py's `[]`). Two
edits in `internal/cli/main.go`:

- `rewriteParserErrorText`: `` `)` can only be used to close a subshell `` →
  `syntax error near unexpected token `)'`.
- `fatalRecoveredParseError`: that same text → `true`, so recovery aborts at the
  `)` instead of continuing. Earlier-line statements (the recovered `'2 3'`)
  still run first, matching bash.

Both fixes verified byte-for-byte against bash 5.3 via the exact probe runShell
logic.

## NOT shell bugs — array__072 + assign-extended__009 (`$SH`-guard artifacts)

Both cases begin with an Oils per-shell skip guard:

```
case $SH in mksh|bash) exit ;; esac          # array__072
case $SH in bash*|mksh) exit ;; esac         # assign-extended__009
```

`scripts/bash-fidelity.sh` runs the **reference** with `SH=bash` (guard fires →
empty output) but runs **ours** with `SH=/ours` (guard does NOT fire → the body
runs). The diff is therefore entirely the guard asymmetry, not a parse error.

Proof (real `bash:5.3`, same non-matching `SH=/ours`):
- array__072 → real bash produces ours's EXACT output (`var: command not found`,
  `a=(1 2 3 4 5)`, `b=()`).
- assign-extended__009 → real bash produces ours's output modulo a *separate*
  invalid-UTF-8 storage detail (`$'\xfe\xff'` stored as `""` vs raw bytes) that
  lives in `../sh` interp, is unrelated to parse errors, and does not change the
  guard verdict.

There is no early parse error in either script, so "extend
`unexpectedTokenAbort`" cannot apply and would produce wrong output (the empty
reference output comes only from the guard `exit`).

### Verified remedy (NOT applied — needs sign-off)

Invoking the drop-in **as** bash — name the binary `bash`, prepend it to `PATH`,
set `SH=bash` for the ours run — makes the `case $SH in bash) exit` guards fire
for the drop-in (it IS bash) while `$SH -c '…'` still resolves to the drop-in
via PATH. With that harness change the entire array / array-assign /
assign-extended corpus shows **0 diffs** (run twice). It is left unapplied
because `SH=bash` is a global change to the shared fidelity harness that drives
the project's headline scoreboard across ALL suites (cases doing `$SH --version`
/ `case $SH in */bash)` could shift), and that warrants explicit review rather
than being bundled with a scoped parse-error fix.

## Verification

- `go test ./...` — green (added `TestRunStrayCloseParenAborts` + two
  `TestPrintBashParseErrorArrayCompatibility` cases; `TestRunNestedBadSubst
  RecoveryContinues` and all `internal/cli` tests still green — no over-abort).
- `make test-bash` — 84/86, unchanged (`glob-bracket`/`quotearray` were already
  failing at baseline; no regression).
- Full oils-corpus differential, pre vs post: array__005 and array-assign__006
  removed from the DIFF set, no new stable divergences (assign-extended__010/__015
  appeared intermittently and are flaky `{…} | grep` stdout/stderr-interleave
  cases — bash == pre == post in isolation).
