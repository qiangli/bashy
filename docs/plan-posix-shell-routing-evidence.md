# POSIX shell routing evidence

## Purpose

Profile B assigns 22 POSIX command names to the shell rather than Bashy's Go
utility provider.  Give each name a stable, command-specific test reference
that proves the CLI selects the shell route.  These tests deliberately do not
claim that the command's full POSIX semantics are conformant; semantic evidence
lives in the `sh` repository and the certification results.

## Routing classes

- `sh`: invoking the CLI with `argv[0] == sh` enables the POSIX parser/runtime
  route and the strict POSIX rules.
- `time`: the parser selects the `time` reserved word and execution does not
  enter the external-command handler.
- `alias`, `bg`, `cd`, `command`, `fc`, `fg`, `getopts`, `hash`, `jobs`,
  `kill`, `read`, `umask`, `unalias`, and `wait`: strict-POSIX intrinsic
  utilities bypass `PATH` and execute in the shell.
- `echo`, `false`, `printf`, `pwd`, `test`, and `true`: strict-POSIX regular
  builtins first require a successful `PATH` lookup, then execute in the shell
  without entering the external-command handler.

## Evidence shape

Add one top-level `TestProfileBRoute<Command>` function per command under
`internal/cli`.  Shared helpers may set up the CLI and make common assertions,
but every ledger row gets its own exact test identifier.  An injected
`AgentOSWireExec` middleware records any external dispatch; every command test
must prove that record stays empty.

For regular builtins, test both sides of the boundary: an empty `PATH` must
produce command-not-found status 127, while an executable marker in `PATH`
must admit the builtin and produce its builtin-specific result without running
the marker or the injected handler.

## Gates

1. Run all 22 routing tests repeatedly.
2. Run them under the race detector.
3. Run `go test -short ./...` with a clean system `PATH`.
4. Cross-build the lean Bashy binary for Windows with `CGO_ENABLED=0`.
