# Sprint 721 / Story 3317: `bashy dag` on a fresh HOME

## Root cause

1. **Startup path.** `bashy dag ...` enters `internal/agentos/agentos.go`
   `case "dag":` -> `runCommandAdapter("yoke", "dag", ...)` ->
   `resolveAdapterScript("yoke", "dag")` (`internal/agentos/adapter_dispatch.go`).
   Outside a bashy source checkout (`cwd` in a shapeware folder,
   `BASHY_ADAPTERS_ROOT` and `GOWORK` unset, and the installed `~/.local/bin/bashy`
   built with `-trimpath` so `runtime.Caller(0)` is non-absolute),
   `adapterCandidateRoots()` returns empty. When `$BASHY_HOME/src/bashy` is absent
   and `adapterSourceFetch` is either absent (the dev-host binary `98a460af6987+dirty`,
   built before `48d7c818`) or fails (offline/sandboxed runs, `-buildvcs=false`
   builds with empty `vcs.revision`, or local/unpushed commits),
   `resolveAdapterScript` falls back to `materializeEmbeddedAdapter("yoke/dag/dag.bsh")`,
   which writes **only** `dag.bsh` into
   `~/.cache/bashy/embedded-adapters/<digest>/yoke/dag/` (`adapterembed` embeds
   `.bsh` scripts only, never `main.go` or `go.mod`).
2. **`go` invocation.** `yoke/dag/dag.bsh` checks
   `bin="${BASHY_ADAPTER_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/bashy/adapters}/yoke/dag"`.
   Finding `[[ ! -x "$bin" ]]` (`stale=1`), line 21 `cd`s into `$here`
   (`.../embedded-adapters/<digest>/yoke/dag`) and executes
   `GOWORK=off CGO_ENABLED=0 run_go build -buildvcs=false -trimpath -o "$bin.tmp.$$" .`.
   Because neither `$here` nor any parent directory under `/tmp/h` contains a
   `go.mod`, host `go` exits 1 with
   `go: go.mod file not found in current directory or any parent directory; see 'go help modules'`,
   and `dag.bsh` prints `$build_log` to stderr and exits 1 before `dag` or any
   target body ever runs. Even when source IS reachable (e.g. running a shapeware
   `dag.md` inside a checkout or after `src.Fetch`), `dag.bsh` still invokes
   `go build` on every fresh `HOME` because the adapter cache defaults under `$HOME`.
3. **Why only a fresh `HOME` (and why `bashy skill add .` worked).** On the normal
   `HOME`, `~/.cache/bashy/adapters/yoke/dag` already exists (`-x "$bin"` is true,
   and missing `$gomod`/`*.go` evaluate false for `-nt "$bin"`, leaving `stale`
   empty so `dag.bsh` `exec`s the cached binary directly; `~/.bashy/src/bashy` was
   also already populated). On a fresh `HOME=/tmp/h` (`BASHY_HOME=/tmp/h/.bashy`),
   both the source pin and `/tmp/h/.cache/bashy/adapters/yoke/dag` are absent,
   forcing `stale=1` and the doomed `go build`. Meanwhile `bashy skill add .`
   dispatches in-process (`coreskills.NewSkillsCmd`) without `runCommandAdapter`.
   Existing tests missed this because `TestStage0StatsAdapterEndToEndParity`
   compiled `./cmd/bashy` without `-trimpath` (leaking the checkout path via
   `runtime.Caller(0)`) and `TestResolveAndRunEmbeddedAdapterEmptyHomeAndCwd`
   pre-staged `cachedDag` in `BASHY_ADAPTER_CACHE`.

## Fix and verification

- Dispatch `bashy dag` in-process via `dispatchDag` -> `runDagInproc` ->
  `runDagDispatch` (`dag.NewDagCmd()` + `sprintCapacityServices()`, which also
  keeps `dag_bashpp.go`'s contracted Bash# body runner and `yoke/cmds/all`'s
  in-process `jq` live), while honoring active `shape` slots and explicit
  `BASHY_ADAPTERS_ROOT` overrides.
- Add `# bashy-inproc-fallback` to `yoke/dag/dag.bsh` (and sync `adapterembed`)
  so direct `runCommandAdapter("yoke", "dag", ...)` calls on a source-less
  embedded fallback skip `adapterSourceFetch` and `exec`
  `BASHY_ADAPTER_INPROC=1 "$bashy" dag "$@"` instead of invoking `go build`.
- Add fresh-HOME regression tests driving a `-trimpath -buildvcs=false` binary
  outside the checkout with a poison `go` on `PATH` and `BASHY_OFFLINE=1`.
