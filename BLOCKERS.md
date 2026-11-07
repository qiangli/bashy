# Open items from the offline `make test` lane

- `examples/outpost-profile` `TestWindowsOutPathConversion` (OS=Windows_NT + fake
  cygpath on Linux): passes on macOS here (`GOWORK=off go test -run
  TestWindowsOutPathConversion ./examples/outpost-profile`), so it could not be
  reproduced locally. Needs the Linux gate output of
  `go test -v -run TestWindowsOutPathConversion ./examples/outpost-profile`
  (the `--out =` value) to diagnose.
- `skills` `TestDeskSkillDocumentedVerbsValid` (`viewer --help=mcp` exit 1): the
  test runs `go run .` in `examples/desk/viewer`, which needs that nested
  module's dependencies; expected to be fixed by the nested `go mod download`
  warm-up. Re-measure on the gate.
- `internal/cli`/`internal/agentos` `TestRemoteInstallFake_*`: the test binary
  now also looks for `../../bin/bashy` (the lane builds `bin/bashy`); not run
  locally because no bin/bashy exists in this workspace.
