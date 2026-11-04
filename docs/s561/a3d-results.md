# S561 A3d results: Windows-only internal/agentos failures

CI run 41794723828 (candidate f1bcd711, windows-latest) failed 5 tests that also failed on baseline d34c29b6.

| Test | Red (cause) | Fix (owning layer) | Green |
|---|---|---|---|
| TestPodmanConfOverrideLinuxPaths | Windows path backslashes are escaped in the TOML conf (`%q`), test built the expectation with raw `"`+Dir+`"` | test expectation uses `fmt.Sprintf("%q", nfthelper.Dir())` (also in TestManagedPodmanNftHelperOnlyForEmbeddedEngine) | Windows x3 PASS |
| TestRepairUnitsMaterializeAsBashSharpShims | go shim body spells the bashy path slash-form (`filepath.ToSlash`); test expected backslashes | test expects `filepath.ToSlash(bashy)` | Windows x3 PASS |
| TestRepairPosixLinksNeverShadowWrapperUnits | test read `name.cmd` expecting the `#!` Bash# unit; the unit file is `name`, `name.cmd` is the launcher | test reads the unit `name`, AND on Windows now also asserts the PATH-resolved launcher `repairShimName(name)` exists and invokes the pinned bashy (coverage strengthened) | Windows x3 PASS |
| TestRepairSuiteTimeoutKilledNeverPasses | watchdog kill reached only the shell; orphan grandchild held TempDir ("file being used by another process") | product: `repair_contain_{windows,unix,other}.go` — suite runs in a Job Object (KILL_ON_JOB_CLOSE); Cancel terminates the whole job; `finish` waits for ActiveProcesses==0 before returning. Unix keeps process-group kill | Windows x3 PASS |
| TestRepairProfileKilledSuiteBlocksPassAndCompletion | same | same | Windows x3 PASS |

No skips, no weakened assertions, no timeout growth.

## Verification

- Windows (ssh `elephant`, windows/amd64, `bashy go` go1.27.2), work only under `~/s561-a3d` (TMP/GOTMPDIR inside it, removed afterwards; `~/s562-fence` untouched):
  `GOWORK=off bashy go test ./internal/agentos -run '<the 5>' -count=3` -> `ok ... 34.955s`, 15/15 PASS
  (e.g. TestRepairSuiteTimeoutKilledNeverPasses 17.40s/0.79s/0.99s; TestRepairProfileKilledSuiteBlocksPassAndCompletion 5.90s/4.31s/4.01s).
- macOS: `GOWORK=off go test ./internal/agentos` -> `ok ... 400.584s`. `GOOS=windows go vet ./internal/agentos ./cmd/bashy` clean.
- F1: cmd/bashy `TestYAMLAgentFenceCompilerRunner` hard-coded `../../../ycode/examples` (umbrella layout); it now resolves the pinned ycode and ycode/examples/genie modules via `go list -m` (before the test redirects HOME, which moves the module cache). All assertions kept. `GOWORK=off go test ./cmd/bashy -run TestYAMLAgentFenceCompilerRunner` -> ok.
- Cert identity (these packages are outside both cert builds): `sh scripts/cert-identity-check.sh docs/s561/candidate-manifest.json HEAD`
  - bash: SAME 7de303511b097dee722652b0df303bbdfaa1e9ae7f6800993a918f2c5f784bf2
  - bashy: SAME ae0f8cbf6eff6bcc9ed56334ce29460725c9be831ca33fbb86a12f61535c84aa
- The KB.md change from the preserved WIP commit was dropped.
- Not run: Linux (no host); the unix path is unchanged in behavior (process-group kill) and compiled/tested on macOS.
