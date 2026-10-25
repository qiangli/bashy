# Connect (Sprint 505, Story 1952)

Add one front-door verb, `bashy connect <host> [-- <cmd> [args...]]`.
The local process only forwards the terminal to the existing outpost SSH
client. Outpost retains host resolution, default users, authentication,
elevation, LAN-direct discovery, and tunnel fallback.

Implementation:

1. Parse the host, `--native`, and an optional command after `--`, preserving
   command arguments verbatim. Register the verb and provide three examples.
2. Invoke the existing meshagent process boundary with inherited standard
   descriptors and propagate the child exit status.
3. Verify parsing, exact transport arguments, terminal descriptors, and exit
   status with a fake transport and no network. Run the touched package tests,
   `go build ./...`, and `CGO_ENABLED=0 GOOS=windows go build ./...`.

Prerequisite discovered during implementation: the pinned outpost SSH CLI
has no TTY command option. Its command path calls buffered `Client.Exec`;
only the no-command path calls `Client.Shell` with a PTY. Neither its CLI
nor its server shell has WSL Ubuntu/native selection. Do not manufacture
unsupported outpost flags or claim fake-transport tests prove these missing
features. Extending that existing interface requires outpost/yoke work and
available dependency pins; permission to expand the workspace scope is pending.

The partial implementation attaches the terminal with `outpost ssh -- HOST`,
using the existing meshagent resolver and inherited standard descriptors.
It deliberately refuses command/native requests rather than degrading to
buffered, non-TTY execution. The current outpost server launches `bashy -i`,
so login-shell startup files also remain a prerequisite (`bashy -il`).

Completion requires the following changes in the existing transport:

- Expose streamed PTY command execution from outpost's SSH CLI (the server
  already supports SSH exec after pty-req). Preserve argument boundaries.
- Select a bashy login shell, and on Windows select WSL Ubuntu when installed,
  with explicit native selection. Keep this policy on the remote side.
- Retain the existing alias, user, authentication and elevation paths.
- Publish/pin those dependency changes before enabling the remaining forms
  here. No sibling dependency, live host, transport or release was modified
  by this partial implementation.

Verification (2026-10-25):

- `go test ./internal/agentos -run 'Test(ParseConnect|Connect)' -count=1 -timeout=90s`: PASS.
- `go test ./internal/agentos -count=1 -timeout=10m`: PASS (272.952s).
- `go build ./...`: PASS.
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...`: PASS.
- Built `cmd/bashy` and invoked `connect --help`: PASS.

The fake outpost test verifies the exact `ssh -- elephant` arguments, all
three terminal descriptors, input and output, and remote exit status 37.
These are local wrapper checks, not evidence of a live remote session or
the missing command/login/WSL behavior. Story 1952 remains incomplete.
