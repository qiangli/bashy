# Sprint 561 A4a: frozen-candidate gate results

Story `9f27d68da431` (#3061), child of A4. Candidate: bashy `f1bcd711`
(sh `74586fde`, coreutils `211789fa`), `GOWORK=off`, Darwin/arm64, go1.27.2.
No code changes; failures are reported, not fixed. The checkout HEAD was
`cc5c0f08`; `git diff f1bcd711 HEAD -- . ':!docs' ':!KB.md'` is empty, so every
build input equals `f1bcd711` (KB.md is another agent's uncommitted note,
untouched, not a build input).

## Headline: the gate is NOT green and NOT complete — host disk exhaustion

`/System/Volumes/Data` had 3.1 GB free at start (100% full, shared with other
agents) and 1.4 GB at the end. Run 1 of `go test ./...` hit
`No space left on device` 46 times (linker/strip/`ld` writes), so its failures
are largely ENOSPC artifacts and are not a candidate verdict. "Run ONCE" was
honoured: I did not rerun, and I did not run `make dist` (12 binaries would
consume the remaining free space and starve other agents on this host).
**Rerun items 1, 2 and 4 on a host with >= 10 GB free.**

## 1. `GOWORK=off go test -timeout 20m ./...` (run once, exit 1, INVALID by ENOSPC)

Log: `/tmp/a4a/gotest.log` (out of git), sha256
`44951ba03c5e7f634ae86d54c51a448afd14db63c91ce459dc60da4d4ed98669`, 1241 lines.
43/43 packages reported: 23 ok, 9 FAIL, 11 no-test-files (A2 baseline: 29 / 3 / 11).
No skips; no timeouts; no panics.

FAIL packages and failing tests (top-level):
- `cmd/bashy` — `[build failed]`: `ld: write() failed, errno=28`. Entire package unmeasured.
- `examples/desk` — TestDeskRegisterScriptsUsePublicShapedAppPath, TestDeskToolsForkAndRollback
- `examples/outpost-profile` — 16 tests; first is `build ./cmd/bashy: exit status 1` (strip/ENOSPC)
- `internal/agentos` (730.7 s) — ACP help/process, CommonOptionsVocabulary, CreateToPostDAGFixture, NotFoundHint x3, ReaderPrefsBudget, ShellOutputReduction x4, TerminalSink x2, **TestRunSignaledIsNonLossy**, ReleaseSbom x2 (`build test binary`, strip/ENOSPC), SynergyDAG x3
- `internal/cli` — AgentTUINativeOverPTY, InteractiveBashPP x2, CLIClassicOptionListings, CLIBashyExplicitBashPPOption, CLIBashPPPOSIXContract, GoSourceExecutableEnvironment, NoExecParsesWithoutRunning
- `native` — TestLauncherPreservesInheritedSignalIgnore
- `tools/elfaudit` — Linux/Darwin SignalSnapshotBuildGate
- `tools/productgo` — ProductRuntimeControls, ProductDownloadedToolchain
- `tools/releaseeligibility` — TestRejectCoreProbeArtifact

Classification vs A2 (`a2-results.md` §4.1):
- F2 `TestCommandDocSourceNames` — **fixed** (passes).
- F4 `TestSrcPinsMatchLinkedModules` — **fixed** (passes).
- F5 `tools/bash53suite` — **fixed** (package ok, incl. BashPPGateInputFixture).
- F3 `TestRunSignaledIsNonLossy` — **still pre-existing** (`run_test.go:51`, same message; not ENOSPC).
- F1 `TestYAMLAgentFenceCompilerRunner` — **unmeasured**: `cmd/bashy` did not build (ENOSPC).
- Every other FAIL above is **NEW relative to A2 but unverified**: those I
  inspected failed on a bashy build/link step with ENOSPC; the rest (cli,
  native, elfaudit, productgo, releaseeligibility, remaining agentos/desk tests)
  all build or run product binaries and are presumed ENOSPC-induced. I did not
  confirm each one individually, so none can be called a real regression or
  cleared. Needs a rerun with disk headroom.

## 2. `GOWORK=off make dist` — NOT RUN (disk blocker above)

No dist hashes recorded. Needs ~2 GB of binaries plus link scratch.

## 3. Candidate manifest — DONE

`GOWORK=off env make build` exit 0 (`elfaudit` and `releaseeligibility` PASS on
bashy), then `GOWORK=off sh scripts/s561-baseline-manifest.sh f1bcd711` ->
`docs/s561/candidate-manifest.json`. Artifact sha256:
- `bin/bash` `b0d19ed071ba4bd93811ba5aba26b8a5bf7aa82e3e920ebf1b720a713d575efc` (same as A1 baseline)
- `bin/bash.real` `26ac0cbcd794a774b8507f063abbb8a86b0cb837b171555b757e90a3e81612c9`
- `bin/bashy` `68c3ddda387e73031ccef05200db30bd5508423d3933d03f2892e9d9606fd19d`
- `bin/bashy.real` absent by Makefile design.

Note: bare `make` in this shell is bashy's in-process make and rejects the
Makefile (`makefile:1: invalid inference rule`); use `env make`.

## 4. Container gate — BLOCKED; native result non-authoritative

`make test-bash-container` not run. Blocker: `podman` 6.1.2 (libkrun) is
installed but `podman machine list` is empty and `podman info` reports
`Cannot connect to Podman`; no VM exists, and creating one needs several GB of
disk that this host does not have. No docker.

Native `PATH=/bin:/usr/bin:... GOWORK=off env make test-bash`: **86 passed, 0
failed, 0 skipped, 0 timed out** (rc 0) — non-authoritative per brief.

## Cleanup
Removed only my own run leftovers in `$TMPDIR` (new `bashy-agentos-commands-*`
and `bashy-coreutils-bin-*`). `bin/` is gitignored and left in place
(~230 MB; delete if space is needed). The shared Go build cache was not touched.
