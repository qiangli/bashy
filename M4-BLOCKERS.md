# M4 delivery evidence and blockers

Sprint: #336; Story: #1574; Story-ID: bdc89715f2c4.

Implemented `shell_open`, `shell_exec`, and `shell_close` with one retained
in-process CLI interpreter per random handle. The warm Unix socket service was
not reused because it constructs a fresh interpreter on each request. Output
spills while writing, with a separate private temporary directory per handle;
close and server cancellation remove those directories.

## Required build gate: missing supplied sibling sources

`go build ./...` fails before compilation:

```
go: github.com/ollama/ollama@v0.0.0-00010101000000-000000000000
(replaced by ../yoke/external/ollama/src): reading
../yoke/external/ollama/src/go.mod: no such file or directory
```

Three checks established the blocker:

1. The exact required `go build ./...` fails with this diagnostic.
2. `go list -e -json ./...` fails with the same module-loading diagnostic;
   inspecting `../yoke/external/ollama/src` confirms it is an empty directory.
3. Even `go build ./cmd/bashy ./internal/agentos/` fails with the same diagnostic.

The root go.mod already points at the provided sibling path. Restoring the
matching Ollama source tree is an environment-provisioning fix, not a source
patch. No verified code patch exists to include for absent third-party sources.
Sibling files were not edited and no remotes were followed. After the workspace
provider restores the dependency, rerun the full requested gate.

## Synthetic-tool effect integration

The supplied yoke/mcp exposes NewServerWithOptions, but Policy.Check requires
names to be present in immutable Atlas or the coreutils registry. Its private
commandEffects function consults only Atlas; Options has no synthetic effect
override. MCP-only registry sentinels allow the dedicated typed tools through
the known-command check and explicitly reject invocation through run_tool.
The shell_exec description declares `exec`, but the yoke audit currently records
an empty effect list for these names. This is the explicitly permitted
"document and keep going" fallback, not a claim of complete effect integration.
A future yoke change should expose server-local synthetic command effects and
use those both for known-name checks and auditing. No speculative sibling patch
was applied or claimed verified.

## Verification

- `go test ./internal/agentos/ -run 'MCP|Session'`: PASS, 32 top-level tests and
  2 subtests (34 pass events), 0 failures, 0 skips. Five new top-level tests cover
  persistence, environment/functions/stdin/status, isolation, closed and unknown
  handles, 1 MiB stdout and stderr, threshold boundaries, and cancellation cleanup.
  All new tests use an in-memory SDK client and yoke server, never a bashy binary.
- `go vet ./internal/agentos/`: PASS, 0 diagnostics.
- `git diff --check`: PASS, 0 whitespace errors.
- Required full build: 0 successful runs, 1 failed full-build attempt; the two
  narrower diagnostic attempts above also failed during module loading.

On macOS, ordinary Bash `pwd` preserves logical `/tmp`; the persistence test
compares its realpath with the realpath of `/tmp` rather than changing shell
semantics to return `/private/tmp`.
