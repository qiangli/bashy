# Plan: the `go` build kind and the Go service declarations (Sprint 434, story 2019 / 5aa131c75f20)

Goal: bring the Go services bashy runs under the shared fork-build-sign
pipeline of `plan-shared-fork-build-pipeline.md`, so each is a
bashy-CI-built, Sigstore-signed build of a pinned, permissive upstream
release tag with no third-party tracking and OTel pointed only at
`127.0.0.1:31415`.

This slice is **PREP ONLY**: declarations, the shared kind, and the docs. No
fork repository is created, no branch or tag is pushed, nothing under
`qiangli/*` is touched. The operator steps are written down in each tool's
README so the follow-up slice is mechanical.

## Mechanism: `builds/_shared/kinds/go`

Three step fragments, used by every Go tool, reading everything from the
shipped `bashy-build/PIN` so no new template variable is introduced:

- **setup.yml** — `actions/setup-go` pinned by commit sha
  (`b7ad1dad…`, v7.0.0) with `go-version-file: src/go.mod`, then a gate that
  reads the upstream `go.mod` (its `toolchain` line, else its `go` line),
  fails unless it equals the declared `GO_TOOLCHAIN`, and exports
  `GOTOOLCHAIN=go$GO_TOOLCHAIN` so every later `go` command uses exactly that
  release whatever the runner image shipped. A pin bump that forgets
  `GO_TOOLCHAIN` fails here rather than building on a drifted toolchain.
- **build.yml** — one `go build` per matrix leg. `GO_CGO` declares, per asset,
  what the **upstream release** does (`ASSET=0|1`; a missing entry fails);
  `GO_BUILD_TAGS`, `GO_BUILD_FLAGS` and `GO_BUILD_ENV` carry the rest of
  upstream's recipe. bashy adds `-trimpath`, `-s -w -buildid=`, and a
  `GO_LDFLAGS` stamp built **only from `PIN`** — never a build timestamp and
  never `git describe`, which is what most upstreams embed and what makes
  their releases unreproducible. Finally the artifact must answer
  `$GO_VERSION_ARG`.
- **audit.yml** — pinned `govulncheck` (`GOVULNCHECK_VERSION`) in source mode
  over `./...`, writing the JSON report as a release asset. The verdict is
  **reachability-based**: a finding whose first trace frame names a called
  function fails the build unless its OSV id is listed with a justification in
  the reviewed `files/vuln-ignore`. Module-only findings are informational.
  Then a CycloneDX SBOM via pinned `cyclonedx-gomod` (`GO_SBOM_VERSION`).

A tool may override one fragment and inherit the rest: `gitea` is `KIND=go`
with its own `build.yml`.

## The declarations

Six tools, each pinned to the release yoke runs today (`yoke/external/<tool>`
or `pkg/fleet/baseline/services/<tool>.yaml` at yoke main), each license
verified **at that tag**:

| tool | upstream | tag | commit | license | notes |
|---|---|---|---|---|---|
| `victoria-logs` | `VictoriaMetrics/VictoriaLogs` | `v1.51.0` | `b6524fb7…` | Apache-2.0 | cgo on linux as upstream |
| `victoria-metrics` | `VictoriaMetrics/VictoriaMetrics` | `v1.147.0` | `3ac6505d…` | Apache-2.0 | same |
| `victoria-traces` | `VictoriaMetrics/VictoriaTraces` | `v0.9.4` | `f96aa3dd…` | Apache-2.0 | same |
| `gitea` | `go-gitea/gitea` | `v28.1.0` | `4ebd5e31…` | MIT | own `build.yml`: pnpm + vite before `bindata` |
| `kopia` | `kopia/kopia` | `v0.23.1` | `72ec08fd…` | Apache-2.0 | the only OTLP **exporter** in the set |
| `zot` | `project-zot/zot` | `v2.1.21` | `6429ce64…` | Apache-2.0 | built without the `ui` tag |

Three decisions worth recording:

1. **gitea needs a frontend build.** Upstream's release binaries carry the
   `bindata` tag, which embeds vite output, so `pnpm install --frozen-lockfile`
   + `pnpm exec vite build` + `go generate -tags bindata` must run first. A
   second step fails the build if pnpm rewrote `pnpm-lock.yaml` anyway. At this
   tag sqlite needs no cgo (`models/db/driver_sqlite_modernc.go` is
   `//go:build !sqlite_mattn`), so `CGO_ENABLED=0` still serves
   `DB_TYPE = sqlite3`, which is what loom uses.
2. **zot is built without `ui`.** `make ui` downloads `zui.tgz` from a *second*
   upstream repository at `ZUI_VERSION := commit-a7feb46` — not a release tag,
   not pinnable to a commit by this pipeline, not representable in the SBOM.
   Dropping the tag removes only the browser UI; the OCI distribution API,
   which is bashy's whole interface to zot, is untouched.
   `buildinfo.BinaryType` is stamped `bashy-no-ui`.
3. **kopia exports on purpose.** yoke's `external/kopia` passes
   `--no-check-for-updates --otlp-trace`, so `otel.env` leaves the SDK enabled
   and pins every endpoint at `127.0.0.1:31415`. Consequently the no-egress
   verdict for all six tools tolerates exactly two destinations: loopback to
   the service's own port, and `127.0.0.1:31415` (yoke's
   `serviceenv.LocalCollectorHostPort`).

## No-tracking and OTel

Every `files/otel.env` installs the same endpoints and protocols as yoke's
`external/internal/serviceenv` `LocalOTLP()` (general plus per-signal traces,
metrics and logs, all `http://127.0.0.1:31415`, `http/protobuf`), so a
bashy-signed binary and a yoke-launched service agree. Five of the six also set
`OTEL_SDK_DISABLED=true` because they link no exporter.

`files/no-tracking.env` carries the per-tool knobs with their upstream source,
and in every case clears `HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY` and pins
`NO_PROXY` to loopback — Go's `net/http` honours those, so an operator shell
could otherwise hand a service an egress path it never asked for. gitea and zot
additionally ship a config baseline (`app.ini`, `config.json`) because that is
where their knobs live; both mirror what yoke seeds.

`files/no-egress-test.sh` is a real runtime gate per tool: a `run.sh` carrying
the tool's documented startup sequence (ending in `exec`, so the server is the
tracer's direct child), run in a fresh Linux network namespace under
`strace -f -e trace=connect,sendto,sendmsg,sendmmsg` via `setpriv`, probed over
loopback, idled, probed again. Any non-loopback attempt, any attempt to a
loopback port other than the two allowed, and any resolver-socket connect fails
the build. macOS uses `sandbox-exec` with everything but those two
destinations denied, and treats any Sandbox deny in the unified log as failure.

## What is NOT added, and why

Both remaining services in the audit are permissive, so neither is a licensing
block. Each needs a `_shared/` decision first, which is a separate slice
because changing the skeleton re-renders `openclaw` and `tuwunel` too:

- **`act_runner`** (MIT) has **no GitHub upstream**: it lives on
  `gitea.com/gitea/act_runner` with binaries on `dl.gitea.com`, which is why
  yoke resolves it with a `binmgr.URLSpec`. The pipeline's provenance gate is
  GitHub-shaped end to end (`actions/checkout` of `UPSTREAM_SLUG`, plus a `pin`
  job requiring a non-draft non-pre-release GitHub release at the tag).
- **`seaweedfs`** (Apache-2.0) tags releases as bare `X.Y` — `4.48` is what
  `seaweedfs.yaml` pins, and the last `v`-prefixed tag is `v3.33`. Three places
  refuse a tag that is not a plain `vX.Y.Z` (`render.sh`, `verify-pin.sh`, the
  `pin` job) and the workflow trigger is `bashy-v*`, so `bashy-4.48-1` would
  not even fire.

**Windows legs are also deferred**, even though gitea ships windows amd64+arm64
and the Victoria components ship windows/amd64. The blocker is the skeleton:
its own steps are bash (`set -euo pipefail`, `. fork/bashy-build/PIN`,
`verify-pin.sh`) and would run under PowerShell on a `windows-*` runner, and
the no-egress harness is built on `unshare`/`strace` and `sandbox-exec` with no
Windows equivalent. Adding Windows means `defaults.run.shell: bash` in
`_shared/bashy-build.yml` plus a Windows no-egress harness.

All three limits are recorded in `builds/README.md` so nothing is silently
missing.

## Acceptance

`builds/render.sh <tool>` renders each new tool, and `builds/check.sh` passes
for all eight tools — the six new ones plus `openclaw` and `tuwunel`, whose
`fork/` trees are byte-identical because `_shared/bashy-build.yml`,
`verify-pin.sh` and `kinds/cargo` were not touched. `check.sh` also runs
`bash -n`, `shellcheck -S warning` on every shipped script and `actionlint` on
every rendered workflow.

## Follow-up (not this slice)

Per tool: create `qiangli/<tool>`, install `fork/` on an orphan `bashy-build`
branch, tag `bashy-<upstream tag>-1`, then repoint the yoke pin
(`external/otel/stack/execstore.go`, `external/loom`, `external/kopia`,
`external/zot`) at the signed fork release and verify it against the signing
identity and issuer in each README.
