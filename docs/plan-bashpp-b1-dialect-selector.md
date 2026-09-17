# Plan: Sprint 98 Story #125 (B1) — the resolved Bash++ dialect/selector model

**Status:** implemented and wired into the cold CLI and warm-session paths.
The original Sprint 98 preparation slice was activated by later integration
work. Sprint 114 established standalone `bash`'s POSIX/Bash++ inertness
profile; Sprint 119 aligns Bashy's effective resolver and runtime with its
existing POSIX parser isolation. Companion design:
`docs/bashpp-posix-superset-syntax.md` in the umbrella.

## Scope

Story #125 asks for one resolved dialect/selector model covering:

- `--bashpp` / `--bash++` (exact aliases, last-flag-wins)
- `--no-bashpp`
- `BASHY_BASHPP=1|0`
- `.bpp` file extension
- binary defaults (`bash` off, `bashy` on)
- effective POSIX policy, with explicit precedence

and an audit of every parser entry path in `internal/cli`.

## Design

`internal/cli/bashpp.go` owns the startup dialect policy. Resolution is a pure
function; `main.go` and `session.go` use it before creating the interpreter.

- `BashPPSelector` is a pure-function input: `Binary` (`bash`/`bashy`),
  `Args` (os.Args-shaped), `LookupEnv` (os.LookupEnv-shaped), `Filename`,
  and the already-resolved `Posix` bool. Taking these as parameters instead
  of reading `os.Args`/`os.Environ()` directly keeps resolution independently
  testable and mirrors `startupPosixForEnv(env []string)`'s existing shape
  in `main.go`.
- `ResolveBashPP` applies the precedence chain
  (`commandLineBashPP` > `envBashPP` > `.bpp` suffix > `Binary.bashPPDefault()`)
  and returns a `BashPPResolution{Enabled, Source, Posix}` after applying the
  POSIX policy. `Enabled` describes effective grammar/runtime, while `Source`
  and `Source.Explicit()` preserve which selector tier won. A `.bpp` filename
  is a selector tier only on Bashy; standalone `bash` requires an explicit
  CLI or environment request.
- `commandLineBashPP` mirrors `commandLinePosixMode`'s scan shape (stop at
  `--`, `-c`, or the first non-flag operand) for the same reason: invocation
  options precede the script path, and once a positional operand appears the
  remaining words are script arguments, not shell flags.
- Startup POSIX always disables Bash++ grammar. Bashy retains POSIX semantics
  regardless of its default or an explicit Bash++ request. Standalone `bash`
  preserves Sprint 114's special inertness profile when the winning selector
  is affirmative: extensions and POSIX differences are both off, matching
  the selector-off, POSIX-off invocation. An explicit off selector or ordinary
  `bash --posix` retains POSIX mode.
- `BashPPResolution.LangVariant()` supplies the effective runtime dialect.
  `ParserOptions(base)` preserves Classic Bash grammar and the POSIX semantic
  profile, never a combined Bash++/POSIX grammar. An explicit `LangPOSIX` base
  also suppresses Bash++.

| Startup selection | Bash++ enabled | Effective POSIX |
|---|---:|---:|
| standalone `bash`, no signal, including `.bpp` | no | off |
| standalone `bash --bashpp`, without POSIX | yes | off |
| standalone `bash --posix`, no signal or explicit off | no | on |
| standalone `bash --posix`, affirmative CLI/environment selector | no | off (Sprint 114) |
| `bashy`, no signal or affirmative selector, without POSIX | yes | off |
| `bashy --no-bashpp`, without POSIX | no | off |
| `bashy`, startup POSIX from argv or environment | no | on |

POSIX application happens once after the other startup options in `newRunner`.
The raw `SHELLOPTS=posix` token is skipped during option import because the
resolver has already incorporated it. All other imported options are retained.
This prevents an imported token from reactivating POSIX in the standalone
inertness profile. Warm-session resolution always uses the Bashy front door.

The interactive front door consults `Runner.LangVariant()` before each
statement rather than its latent `Dialect()`. Effective POSIX is translated
back to Classic Bash grammar to preserve the drop-in's arrays and parameter
extensions. A live Bash++ toggle remains latent while POSIX is on; turning
POSIX off exposes that selected dialect again for subsequently parsed input.
Re-enabling POSIX suppresses the extended grammar again. This applies to both
readline and the plain-terminal fallback without changing their startup
`PosixMode` behavioral profile.

An explicitly requested Go unit on Bashy still receives its existing POSIX
refusal before shell parsing. Recognizing that unit uses the winning selector
only for the refusal; it does not enable the grammar or runtime.

## Parser entry-path audit (recorded in the file's doc comment)

Every site in `internal/cli` that currently pins a `syntax.LangVariant`:

| Site | Role | User dialect? |
|---|---|---|
| `main.go` `run()` + `bashyParseOpts` + `parseOnce` + the `-c` parse | primary script/-c/stdin execution path | yes |
| `interactive.go` `runInteractive` (~62,105,141) | readline-backed interactive REPL | yes |
| `forced_interactive.go` `runForcedInteractiveExec` (~224) | non-TTY `bash -i` emulation | yes |
| `forced_interactive.go` `runnerExpand` (~191) | synthetic `${...}` prompt/HISTFILE bookkeeping | no — not user source |
| `session.go` `RunSessionCommand` (~98) | live-session socket command path | yes |
| `main.go` `completeStmtBeforeLine` (~3994) | diagnostic-only re-parse for error formatting | no — intentionally bash-fixed |
| `main.go` `registerDefaultFuncs` (~831), `importBashFuncs` (~873) | preamble/inherited-function parsing | no — always plain Bash by construction |
| `main.go` `BASH_EXECUTION_STRING` assignment (~2621) | internal bookkeeping | no — not user source |

## Testing

`internal/cli/bashpp_test.go` — table-driven precedence tests (all four
tiers, alias equivalence, last-flag-wins, argv scan boundaries at `--`/`-c`/
the script operand), both POSIX policies, retained source/explicit metadata,
grammar activation and rejection, and POSIX parsing semantics. The Unix
front-door tests in `bashpp_posix_contract_unix_test.go` drive both compiled
entry points through command/file parsing, `eval`, and sourcing, with selector
aliases, precedence, ordering and environment controls. The warm-session tests
in `bashpp_posix_session_test.go` check runtime dialect and `eval` isolation.
The existing Sprint 114 byte-inertness and Go-source refusal tests remain
independent regression guards. Release gates run on the approved remote hosts.
`TestInteractiveBashPPPOSIXGrammarContract` verifies effective live grammar,
Classic Bash arrays, rejection under POSIX, and activation after disabling
POSIX through both terminal paths.
