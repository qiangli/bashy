---
id: 117bbf2cac2e
kind: bug
title: 'dag: every target fails on a fresh HOME with ''go: go.mod file not found'' before the body runs'
seq: 3317
status: done
priority: p1
labels:
    - dag
    - fresh-home
created: 2026-11-25T16:03:27.443249Z
assignee: agy-gemini4-carbon
sprint: 721
sprint_id: 69fc2c51-8faf-58e7-83ac-f27007837ae7
sprint_title: 'Shapeware pilot on form: spec v0.1 layout, lifecycle, one-step launch'
closed: 2026-11-25T18:27:15.191395Z
closed_by: claude-opus5.5
---

Seen 2026-11-25 (umbrella Sprint 712, testing the shapeware lifecycle dag.md). Repro on the dev box:
  mkdir -p /tmp/h && cd <folder with a trivial dag.md> && HOME=/tmp/h BASHY_HOME=/tmp/h/.bashy BASHY_HINTS=off bashy dag -f dag.md <target>
  -> go: go.mod file not found in current directory or any parent directory; see 'go help modules'
The target body never runs (an `echo` target prints nothing). With the normal HOME the same dag.md runs fine, and `bashy skill add .` works under the fresh HOME. So `bashy dag` itself invokes go during startup when HOME is fresh.
Same failure class as the 11-24 field defect "D1: go.bsh does not build on a fresh HOME" (umbrella Sprint 712 premise). Shapeware makes dag.md the lifecycle every agent runs (source/build/test/install/run), so a fresh install cannot run its own lifecycle until this is fixed.
Next: root cause first (what go invocation, from which dag startup path, and why only a fresh HOME), then a fix and a fresh-HOME regression test.

Root cause (2026-11-25):
1. Startup path: `bashy dag` dispatches through `internal/agentos/agentos.go` `case "dag":` -> `runCommandAdapter("yoke", "dag", ...)` -> `resolveAdapterScript("yoke", "dag")`. Outside a bashy checkout (`cwd` in the shapeware folder, `BASHY_ADAPTERS_ROOT`/`GOWORK` unset, installed `-trimpath` binary so `runtime.Caller(0)` is non-absolute), `adapterCandidateRoots()` is empty. With `$BASHY_HOME/src/bashy` absent and `adapterSourceFetch` absent (dev-host `98a460af6987+dirty`, pre-`48d7c818`) or failing (offline/sandboxed runs, `-buildvcs=false`, or unpushed commits), `resolveAdapterScript` falls back to `materializeEmbeddedAdapter("yoke/dag/dag.bsh")`, writing only `dag.bsh` into `~/.cache/bashy/embedded-adapters/<digest>/yoke/dag/` (`adapterembed` embeds `.bsh` wrappers only, no `main.go` or `go.mod`).
2. `go` invocation: `yoke/dag/dag.bsh` checks `bin="${BASHY_ADAPTER_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/bashy/adapters}/yoke/dag"`. Seeing `[[ ! -x "$bin" ]]` (`stale=1`), line 21 runs `(cd "$here" && GOWORK=off CGO_ENABLED=0 run_go build -buildvcs=false -trimpath -o "$bin.tmp.$$" .)` inside the source-less embedded cache directory under `/tmp/h`, which fails with `go: go.mod file not found in current directory or any parent directory; see 'go help modules'` before `dag` or any target body runs.
3. Why only a fresh HOME: under the normal HOME, `~/.cache/bashy/adapters/yoke/dag` already exists (`-x "$bin"` is true and missing `$gomod`/`*.go` test false for `-nt "$bin"`, so `stale` stays empty and `dag.bsh` `exec`s `$bin`; `~/.bashy/src/bashy` was also already present), whereas `bashy skill add .` dispatches in-process (`coreskills.NewSkillsCmd`). Existing E2E tests missed this because `TestStage0StatsAdapterEndToEndParity` built `./cmd/bashy` without `-trimpath` and `TestResolveAndRunEmbeddedAdapterEmptyHomeAndCwd` pre-staged `cachedDag`.
4. Even when the fetch works (clean release build, online), the designed path clones the whole bashy repository and provisions Go before the first target of ANY project's dag.md runs. The fetch error itself prints only under `BASHY_ADAPTER_DEBUG`.

Classification: product defect (fresh-install dispatch of a runtime verb).

Decision (operator, 2026-11-25, Sprint 721 g2): "always do the right way; do not assume bashy only works for bashy". dag.md is the lifecycle runner for arbitrary projects (shapeware apps, user folders), so `bashy dag` must run self-contained from the shipped binary: no bashy source fetch and no Go toolchain. `bashy dag` therefore dispatches in-process by default (the binary already links yoke pkg/dag); the standalone `yoke/dag` adapter remains for standalone builds, an explicit `BASHY_ADAPTERS_ROOT` and active shape slots. This supersedes the Sprint 692 adapter routing for `dag` only.
