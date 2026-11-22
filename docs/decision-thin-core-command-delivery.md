# Decision: thin core, commands built on demand (P5, Sprint 692)

Story `aa693f4aa0e6` (#591). Recorded 2026-11-22.

## Decision

1. **The scratch/air-gapped image ships the thin core.** It is the static
   `bashy_scratch` build of `cmd/bashy`: Bash 5.3, `--posix`, Bash#, the
   in-process Coreutils and the internal bashy verbs. Commands in the five
   command folders (`optional/`, `toolchain/`, `yoke/`, `custom/`,
   `examples/`) are not linked into it. `TestBashyImportBoundary`
   (`cmd/bashy/import_boundary_test.go`) enforces this on Linux, macOS,
   Windows and the `bashy_scratch` and `bashy_engines bashy_obs` profiles.
2. **Commands are built on demand.** `bashy <verb>` runs
   `<folder>/<entry>/<entry>.bsh`, which builds `main.go` into
   `$BASHY_ADAPTER_CACHE` (default `~/.cache/bashy/adapters`) on first use
   and execs the cached binary after that. A cold build needs two things on
   the host: a Go toolchain (host `go`, or the toolchain adapter through
   `BASHY_ADAPTER_INPROC=1 bashy go`) and the adapter source. The source
   comes from the fetched source pin (`~/.bashy/src/bashy`), a checkout, or
   the embedded adapter scripts (`resolveAdapterScript`), plus each module's
   pinned dependencies.
3. **Fallback: prebuilt per release.** A host that cannot build is
   air-gapped, has no toolchain, or has no module cache. For those hosts the
   fallback is per-release prebuilt command binaries, staged into
   `$BASHY_ADAPTER_CACHE` (or a shaped slot) next to the image. This needs
   no product change: the adapter execs a cached binary that is newer than
   its `main.go`/`go.mod`. This fallback is not built for rc.40; it is
   follow-up work if an air-gapped consumer needs adapted verbs.

What still runs offline in the image: everything in the core, including the
in-shell applets that `internal/agentos` registers in-process through
`github.com/qiangli/yoke/cmds/all` (for example `jq`, `tree`, `tar` inside
a script). What does not run offline without a prebuilt cache: a front-door
`bashy <adapted-verb>` on its first use.

## Measurements

Dev box (darwin/arm64), `go build -trimpath ./cmd/bashy`, unstripped:

| | before (bashy `42838686`) | after P5 |
|---|---|---|
| `go list -deps ./cmd/bashy` | 2,276 | 2,275 |
| binary bytes | 299,387,298 | 299,370,562 |
| `go list -deps -tags bashy_cert_base ./cmd/bashy` | 915 (base profile) | 2,275 (profile removed; the tag is now inert) |

After P5, linux/amd64 has 2,273 packages and windows/amd64 has 2,266.

First use of an adapter on the dev box, from a checkout
(`BASHY_ADAPTERS_ROOT`), with the module cache warm:

| verb | cold adapter, cold `GOCACHE` | cold adapter, warm `GOCACHE` | warm (cached binary) | cached binary bytes |
|---|---|---|---|---|
| `bashy tree --help` | 14.3 s | 2.2 s | 0.12 s | 76,784,978 |
| `bashy kb --help` | 18.3 s | 3.4 s | 0.43 s | 112,244,274 |

On a bare host, first use also downloads the Go toolchain and the module
graph. That network time is not measured here. It is the main argument for
the prebuilt fallback on air-gapped and bandwidth-limited hosts.

## Why the package count barely moved

P5 deletes only what is provably dead. Proof comes from `go list -deps`
for package removals and from linker symbol tables, built with inlining
off, across darwin, linux, windows and the tag profiles, for function
removals. What remains linked is reachable from live code:

- internal verbs that stay in bashy. `dks` scope pulls `k8s.io/client-go`
  (208 packages unique to that edge). `app` pulls `yoke/pkg/webconsole`
  (207). `ycode` pulls `ycode/pkg/ycodecli` (105). The rest of the AgentOS
  surface is `steward`, `conductor`, `bus`, `skill`, the fleet registry,
  `mcp`, `inspect`, `self` and similar.
- the help/MCP schema projection (`cobra_schema.go`). It builds the
  in-process cobra tree of each adapted verb so that `--help --format
  skill|mcp` and `mcp` report its flags (about 21 packages).
- the in-shell applet registration `yoke/cmds/all` (31 packages). Inside a
  bashy script, bare `jq`, `tree`, `tar` and the rest still run in-process
  through the Coreutils ExecHandler.

Removing any of these first needs a switch step: adapter-provided schema
metadata, an in-shell rung that routes those names to their adapters, or
moving the internal verb out. Each is a behaviour change, not a dead-code
deletion, so none is part of this contract step.

## Removed in P5

- The `bashy_cert_base`/`bashy_core` profile: `cmd/bashy/main_core.go`,
  `internal/core` (the `internal/core` → `yoke/pkg/fleet` path, the
  23-package yoke graph in that profile), the `cert_requires_*` guards, the
  core-profile tests, the `releaseeligibility` tag branch, and the
  `bashy_cert_base` tags in `dag.md` `test-startup-linux` and
  `scripts/cert-closure.py`. `bsh` (`bashsharp/cmd/bsh`) is the certified
  thin shell.
- In-process dispatchers that no case calls (fleet-family `dispatch*`,
  `dispatchService`, `applyPodmanHelperEnv`, `serveMCPShells`).

## Open risks for the rc.40 candidate gate

- `test-startup-linux` now measures the full product. Its thresholds
  (init total ≤ 15 ms, max ≤ 5 ms) were met by the base profile. The full
  binary measured 19–45 ms init total on the dev box.
- `smoke-airgap` rows that call adapted front-door verbs need a toolchain
  or a prebuilt cache inside the image (see decision 3).
- Conversion parity gaps found while proving deadness (P4 follow-ups, not
  deleted here):
  - `runDagDispatch` mounted the sprint capacity services, but the
    `yoke/dag` adapter does not.
  - `adaptKubeEnvironment` (kubectl path/env conversion) has no copy in
    `custom/kubectl`.
