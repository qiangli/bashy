# S561 story #3052 (6f172feae90e): A2 fresh-run failures F1-F5

Baseline: frozen tree, `GOWORK=off`, Darwin/arm64, go1.27.2. Red = observed
before the change on the same tree; green = after.

| ID | Test | Class | Disposition | Commit |
| --- | --- | --- | --- | --- |
| F2 | agentos `TestCommandDocSourceNames` | metadata (stale test) | fixed | 03d0ace8 |
| F3 | agentos `TestRunSignaledIsNonLossy` | test-env | fixed (hermetic probe) | 0ce2d9aa |
| F4 | agentos `TestSrcPinsMatchLinkedModules` | metadata | fixed | a079c31d |
| F5 | bash53suite `TestBashPPGateInputFixtureGetsExplicitSelector/0` | test-env | recorded, no change | - |
| F1 | cmd/bashy `TestYAMLAgentFenceCompilerRunner` | setup | prerequisite recorded | - |

## F2
Red: `Names()` had extra `desk`. `skills/bashy/commands/desk.md` ships and
`skills/managed_skills_test.go` already lists `desk`; the agentos expected list
was stale. Not a registered-vs-documented contract defect, so no routing to A1b.
Fix: add `desk` to the expected list. Green: test passes.

## F3
Red: `signaled=true` not reported. Cause: the test ran `sh` from PATH, and on
this host `$HOME/bin/sh` is a Go shell that exits 137 rather than dying by
signal. `procStatus` (run_unix.go) is correct: with PATH=/bin:/usr/bin:... the
unchanged test passed, and `/bin/sh -c 'kill -KILL $$'` signals. This is the
CLAUDE.md "wrapper shim in PATH" gotcha. Fix: probe uses `/bin/sh`; assertions
unchanged. Green on the default PATH.

## F4
Red: `yoke pin is out of sync with ... 89df473c6035`. Only `yoke` was stale in
`srcModulePins`. Fix: full SHA `89df473c6035d6b07471a0267e41fdb466d939a0`
(module cache `.info` VCS origin). The preserved hunk `8f20a0f8` was reviewed and
NOT used: it pins older yoke/coreutils/sh revisions (4895bc9f, a7e78d74,
f8ee4168) that do not match current go.mod, so it would not fix this failure.
go.mod unchanged. Green: all four pins check.

## F5 (no change)
Red reproduces intermittently (2 of 5 runs failed, always subtest `/0`, the first
to execute the freshly written `#!/bin/sh` launcher; `/1` passes in ~0.08s).
Cause: first exec of a newly created script on this host takes ~0.6-1.4s cold vs
~0.03s warm (measured outside the harness with a bare `#!/bin/sh; cat` script and
a copied `cat`), against the test's fixed 1s fixture timeout. Harness logic is
correct (passes when exec is not cold). Classified test-env. Per rules no timeout
increase or skip. Mitigation, if wanted, belongs to the owner (e.g. warm-up exec
before the timed run); A4 should re-observe on the final candidate.

## F1 (no code)
Prerequisite: `cmd/bashy/polyglot_agent_test.go:63` reads
`<checkout>/../../../ycode/examples/agent.yaml` and `genie/agent.yaml`, i.e. a
sibling `ycode` checkout next to the `workspaces/<name>` directories
(`workspaces/ycode`). Absent in this weave layout; run the test where it exists.

## Gate
See final run below (`GOWORK=off go test ./...`).
