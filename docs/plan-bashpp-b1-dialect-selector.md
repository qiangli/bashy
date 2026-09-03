# Plan: Sprint 98 Story #125 (B1) — the resolved Bash++ dialect/selector model

**Status:** implemented as a preparation slice, inert. **Not wired into
`main.go`** — activation is held for the parser/runtime integration story
(B2/B3, #126). Companions: `docs/bashpp-posix-superset-syntax.md` (the design
of record, in the umbrella `docs/`) and
`docs/bashpp-phase-b-shared-file-delta.md` (the `sh`-side inert-insertion
precedent this slice follows the same shape as).

## Scope

Story #125 asks for one resolved dialect/selector model covering:

- `--bashpp` / `--bash++` (exact aliases, last-flag-wins)
- `--no-bashpp`
- `BASHY_BASHPP=1|0`
- `.bpp` file extension
- binary defaults (`bash` off, `bashy` on)
- POSIX/certification refusal, with explicit precedence

and an audit of every parser entry path in `internal/cli`, without making
Bash++ reachable from `main.go`.

## Design

`internal/cli/bashpp.go` is a new, self-contained file with zero external
callers (mirroring how `sh/bashpp_nodes.go` et al. shipped "provably inert"
in Sprint 97 — the proof is structural: nothing constructs a
`BashPPSelector` or calls `ResolveBashPP` outside `bashpp_test.go`).

- `BashPPSelector` is a pure-function input: `Binary` (`bash`/`bashy`),
  `Args` (os.Args-shaped), `LookupEnv` (os.LookupEnv-shaped), `Filename`,
  and the already-resolved `Posix` bool. Taking these as parameters instead
  of reading `os.Args`/`os.Environ()` directly keeps resolution independently
  testable and mirrors `startupPosixForEnv(env []string)`'s existing shape
  in `main.go`.
- `ResolveBashPP` applies the precedence chain
  (`commandLineBashPP` > `envBashPP` > `.bpp` suffix > `Binary.bashPPDefault()`)
  and returns a `BashPPResolution{Enabled, Source}`.
- `commandLineBashPP` mirrors `commandLinePosixMode`'s scan shape (stop at
  `--`, `-c`, or the first non-flag operand) for the same reason: invocation
  options precede the script path, and once a positional operand appears the
  remaining words are script arguments, not shell flags.
- Certification refusal: the design of record states certification uses
  "the standalone `bash` binary with `--posix` and without a Bash++
  selector," and that a certification-profile invocation carrying a Bash++
  flag "must fail clearly." `bashy --posix --bashpp` is a separately
  documented supported combined mode. The two are reconciled by keying the
  refusal on `Binary == BashPPBinaryBash && Posix && Enabled`, regardless of
  which tier turned Bash++ on (CLI flag, env var, or `.bpp` extension all
  leak extended grammar into a certification-labeled invocation equally) —
  never on `Binary == BashPPBinaryBashy`. Refusal returns
  `*BashPPCertificationError` rather than silently downgrading, so a caller
  cannot accidentally run the certification profile with extended grammar
  live.
- `BashPPResolution.LangVariant(base syntax.LangVariant)` composes with
  `main.go`'s existing `bashyParseOpts` shape (`LangBashPP` replacing the
  base variant, `PosixMode` staying orthogonal) — the future B2 integration
  point, not built yet.

## Parser entry-path audit (recorded in the file's doc comment)

Every site in `internal/cli` that currently pins a `syntax.LangVariant`:

| Site | Role | B2 candidate? |
|---|---|---|
| `main.go` `run()` (~2529) + `bashyParseOpts` (~2464) + `parseOnce` (~2654) + the `-c` parse (~2708) | primary script/-c/stdin execution path | **yes — the intended integration point** |
| `interactive.go` `runInteractive` (~62,105,141) | readline-backed interactive REPL | yes |
| `forced_interactive.go` `runForcedInteractiveExec` (~224) | non-TTY `bash -i` emulation | yes |
| `forced_interactive.go` `runnerExpand` (~191) | synthetic `${...}` prompt/HISTFILE bookkeeping | no — not user source |
| `session.go` `RunSessionCommand` (~98) | live-session socket command path | yes |
| `main.go` `completeStmtBeforeLine` (~3994) | diagnostic-only re-parse for error formatting | no — intentionally bash-fixed |
| `main.go` `registerDefaultFuncs` (~831), `importBashFuncs` (~873) | preamble/inherited-function parsing | no — always plain Bash by construction |
| `main.go` `BASH_EXECUTION_STRING` assignment (~2621) | internal bookkeeping | no — not user source |

## Why not wire `main.go` now

Sprint 97's precedent (`docs/bashpp-phase-b-shared-file-delta.md`) landed the
whole P1 typed-AST design as inert new files first, so the shared-file edits
could be reviewed and merged before anything could execute them. This slice
follows the same shape one layer up: the resolver is complete and unit
tested, but plugging its result into `run()`'s `lang` variable, the
interactive REPL's `Lang`, and the session command path is B2/B3's job
(#126), once the parser dispatch it would select actually exists.

## Testing

`internal/cli/bashpp_test.go` — table-driven precedence tests (all four
tiers, alias equivalence, last-flag-wins, argv scan boundaries at `--`/`-c`/
the script operand), certification-refusal tests (CLI, env, and extension
triggers, plus the `bashy`/`--no-bashpp` non-refusal cases), and small
direct tests for `Explicit()`, `LangVariant()`, and the error message's
actionable content.
