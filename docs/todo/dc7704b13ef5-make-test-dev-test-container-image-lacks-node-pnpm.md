---
id: dc7704b13ef5
kind: bug
title: 'make test: dev-test-container image lacks node/pnpm, so test-meet-spa-fresh fails closed on every host'
seq: 3157
status: todo
priority: p1
labels:
    - test
    - ci
created: 2026-11-07T03:33:16.559352Z
---

Found on 2026-11-06 while gating v1.0.0-rc.27-dev. tools/dev-test-container/Containerfile (Ubuntu, golang-go only) has no node or pnpm. make test runs test-meet-spa-fresh as its first prerequisite, and that check exits 2 ('check requires node and pnpm ... but none is available'), so make test-self-container cannot pass anywhere. Fix: install pinned node plus pnpm (corepack) in the image, or provision them in the lane. Then re-run make test-self-container on the Windows test host's rootful gate machine.

## Progress (2026-11-07)
- fa2c70f0 + ba9d7929: bashy-provisioned pnpm (pnpm_config_* env) and engines build deps. On the rootful Linux gate, every make test step before go test passes.
- Run #40: warm-up of nested example modules and bashy Go SDK; repair fixtures honour the cache base (product fix); dev-install tests find bin/bashy.

Open items, to re-measure on the gate:
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
