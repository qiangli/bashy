# Report: every `bashy commands` verb is supported (+ unit/e2e tests)

**Goal.** Ensure every command verb listed by `bashy commands` actually
dispatches to a real handler — no `No such command` / `not in this build` /
`No such file` at runtime — and lock it with a unit test **and** an e2e test,
run on **macOS and Windows**.

## What "supported" means

`bashy commands` advertises three classes of command; each must be backed by a
real handler:

| Class | Backed by | Verified against |
|-------|-----------|------------------|
| **builtins** (`cd`, `export`, `read`, …) | interpreter builtin set | `interp.BuiltinNames()` |
| **coreutils** (`ls`/`grep`/`sed`/… + code-intel `ast` + `graph`) | in-process `tool.Lookup` registry | `tool.Lookup(name) != nil` |
| **front-door verbs** (`weave`/`sprint`/`podman`/`ollama`/`docker`/`gh`/…) | dispatch switch + engine ladder | verb has a synopsis; live `--help` / feature probe |

## Bug found and fixed

`docker` is listed by `bashy commands` as an alias for the podman engine, but
had **no dispatch handler** — `bashy docker …` fell through and errored
`docker: No such file or directory`. Fixed with a shared, testable alias
normalizer applied in **all three** engine build variants:

- **`internal/agentos/engines_common.go`** (new, non-build-tagged):
  `engineAlias("docker") → "podman"`, everything else unchanged.
- **`engines_stub.go`** (lean default), **`engines_full.go`**
  (`-tags bashy_engines`), **`engines_windows.go`** — each `dispatchEngine`
  now normalizes its arg through `engineAlias` before the `podman`/`ollama`
  switch, so `bashy docker …` reaches the podman engine on every build.

This is the same regression class as the earlier `bashy podman`/`ollama`
"not in this build" fix (lean binary now walks the exec-never-link dispatch
ladder instead of erroring).

## Tests

### Unit — `internal/agentos/commands_supported_test.go`

Pure, fast, cross-platform (runs identically on macOS/Windows CI, no exec):

- `TestAllListedCommandsAreSupported` — walks the actual catalog
  (`commandsCatalog()` under `BASHY_AGENTIC=1`) and asserts **every** listed
  builtin ∈ `interp.BuiltinNames()`, **every** coreutils name resolves via
  `tool.Lookup`, and **every** front-door verb (visible + hidden) has a
  synopsis (a verb with no synopsis almost always means no dispatch handler).
- `TestDockerAliasIsHandled` — pins the fix: `docker` is a listed verb and
  `engineAlias("docker") == "podman"`, while `podman`/`ollama`/`gh` pass
  through unchanged.

### E2E — `internal/agentos/commands_e2e_test.go` (`//go:build e2e`)

Builds the **real** `bashy` binary on the current OS (or uses a prebuilt via
`$BASHY_E2E_BIN`), reads `bashy commands --json`, and confirms every listed
command dispatches:

- **coreutils** — each listed tool is recognized+available per the binary's
  own feature report, plus a live pipeline smoke
  (`printf … | grep | tr | wc`) proves the in-process userland actually runs.
- **native + engine verbs** (`weave`/`sprint`/…/`podman`/`ollama`/`docker`) —
  really invoked via side-effect-free `--help`, asserting none emit an
  unsupported signal (`no such command` / `not in this build` /
  `rebuild with -tags` / `no such file or directory` / `command not found`).
- **download/passthrough verbs** (`gh`/`act`/`rclone`/`loom`/`zot`/… /
  `go`/`cmake`/`clang`) — dispatch recognition via the feature report, so the
  test never pulls hundreds of MB of upstream tooling.

Probe-design notes (why the first cut false-positived): `--help` is **not**
universal — path-taking tools (`ast symbols`) treat `--help` as a path
argument, and `time` is a shell keyword — so per-tool coreutils checks use the
feature report, not `--help`; and `runBashy` feeds **empty stdin** so
stdin-reading tools (`cat`/`wc`/…) never hang.

## Results

| Check | macOS (darwin/arm64) | Windows (windows/amd64) |
|-------|----------------------|--------------------------|
| Unit tests | ✅ PASS (`go test ./internal/agentos`) | ✅ PASS in `windows-latest` CI; `go vet` clean cross-compiled |
| E2E test | ✅ PASS (`go test -tags e2e …`, 2.2 s) | ✅ compiles clean cross-compiled; wired into `windows-latest` CI |
| `gofmt`/`go vet` | ✅ clean | ✅ clean |

### Windows coverage mechanism

`.github/workflows/test.yml` already runs the matrix on `windows-latest`; the
untagged unit tests run there today. A new **E2E step** now runs
`go test -tags e2e -run TestE2EAllListedCommandsDispatch ./internal/agentos`
on **all three OSes**, so the real binary is built and every listed verb is
dispatch-probed on a genuine Windows runner on every push.

A one-off manual run on the LAN Windows hosts (`lj2ivy`, `puppy` — both online
`windows/amd64` over the mesh) was attempted but blocked: elevation had
expired and re-elevation needs an interactive OS password (no TTY / no
`$OUTPOST_SSH_PASSWORD` in this headless session). The cross-compiled
`bashy.exe` + e2e test binary were staged and the test grew a `$BASHY_E2E_BIN`
escape hatch (run the dispatch e2e against a shipped binary, no Go/source on
the host) for whenever a live host run is wanted. CI on `windows-latest` is
the durable, credential-free equivalent and covers the same assertions.

## Files

- **new** `internal/agentos/commands_supported_test.go` — unit tests
- **new** `internal/agentos/commands_e2e_test.go` — e2e test (`-tags e2e`)
- **new** `internal/agentos/engines_common.go` — `engineAlias` (shared)
- **mod** `internal/agentos/engines_{stub,full,windows}.go` — normalize through `engineAlias`
- **mod** `.github/workflows/test.yml` — e2e dispatch step on all 3 OSes
