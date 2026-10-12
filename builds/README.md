# builds/ — bashy verified builds (fork-build-sign pipeline)

bashy runs a third-party tool only from a **bashy-CI-built,
Sigstore-keyless-signed build of a pinned, permissive upstream release tag**,
with no third-party tracking and OTel pointed only at bashy's local collector
`127.0.0.1:31415` (umbrella `docs/bashy-service-supply-chain-policy.md`,
rules 1 and 2).

Every tool uses the same pipeline. A tool is **one declaration** (`tool.env`)
plus only its genuinely tool-specific files. `render.sh` generates the tree
that is installed as the orphan `bashy-build` branch of
`github.com/qiangli/<tool>`. `check.sh` proves the committed trees still
match their declarations.

```
builds/
  render.sh                 render.sh TOOL [OUT]  -> builds/TOOL/fork (generated)
  check.sh                  regenerate-and-diff gate (+ bash -n, shellcheck,
                            actionlint, the shared verdict's self-test)
  _shared/
    bashy-build.yml         workflow skeleton: pin -> build matrix -> [audit] -> release
    verify-pin.sh           provenance gate, shipped into every fork
    egress-verdict.sh       THE strace verdict, shipped into every fork that
                            declares SHARED_FILES (sourced by no-egress-test.sh)
    egress-verdict-test.sh  its self-test; check.sh runs it (not shipped)
    kinds/cargo/            setup.yml / build.yml / audit.yml steps for Rust tools
    kinds/go/               the same three for Go tools
  <tool>/
    tool.env                THE declaration (shipped verbatim as bashy-build/PIN)
    files/                  tool-specific files, copied into bashy-build/
    setup.yml build.yml audit.yml   optional step fragments; each overrides the kind's
    README.md               knob tables, upstream evidence, binmgr pin
    fork/                   GENERATED — never edit by hand
```

## Tools

| tool | upstream | pinned tag | license at the tag | kind | who runs it |
|---|---|---|---|---|---|
| `openclaw` | `openclaw/openclaw` | `v2026.10.2` | see its README | `custom` | the OpenClaw gateway |
| `tuwunel` | `matrix-construct/tuwunel` | `v1.9.4` | Apache-2.0 | `cargo` | the Matrix homeserver |
| `victoria-logs` | `VictoriaMetrics/VictoriaLogs` | `v1.53.0` | Apache-2.0 | `go` | yoke `external/otel/stack` |
| `victoria-metrics` | `VictoriaMetrics/VictoriaMetrics` | `v1.154.0` | Apache-2.0 | `go` | yoke `external/otel/stack` |
| `victoria-traces` | `VictoriaMetrics/VictoriaTraces` | `v0.12.0` | Apache-2.0 | `go` | yoke `external/otel/stack` |
| `gitea` | `go-gitea/gitea` | `v28.1.0` | MIT | `go` (own `build.yml`) | yoke `external/loom` |
| `kopia` | `kopia/kopia` | `v0.23.1` | Apache-2.0 | `go` | yoke `external/kopia` |
| `zot` | `project-zot/zot` | `v2.1.21` | Apache-2.0 | `go` | yoke `external/zot` |

Each pin is the release yoke runs today, read from `external/<tool>` or
`pkg/fleet/baseline/services/<tool>.yaml` — except the three Victoria
components, whose pins **lead** yoke (`external/otel/stack/execstore.go` still
names `v1.147.0` / `v1.51.0` / `v0.9.4`). They were bumped to the latest stable
upstream tags to clear every REACHABLE `govulncheck` finding on the fork CI;
yoke's flip to the signed fork releases follows once that CI is green and the
bashy tags are cut. Every license above was checked at the pinned tag, not at
HEAD.

### Not yet addable

Two services in the audit are **blocked**, and neither is a licensing problem —
both are permissive (`seaweedfs/seaweedfs` Apache-2.0, `gitea/act_runner` MIT).
Each needs a pipeline decision first, so no declaration exists for them:

- **`act_runner`** — there is **no upstream on GitHub**. Its home is
  `gitea.com/gitea/act_runner` and its binaries are published on
  `dl.gitea.com` (which is why yoke resolves it with a `binmgr.URLSpec`, not a
  `GitHubSpec`). The shared pipeline's provenance gate is GitHub-shaped end to
  end: `actions/checkout` of `UPSTREAM_SLUG`, and a `pin` job that requires a
  non-draft, non-pre-release **GitHub** release at the tag. Supporting it means
  teaching the skeleton a second forge, which is a `_shared/` change, not a
  declaration.
- **`seaweedfs`** — its release tags are bare `X.Y` (`4.48`, the version
  `pkg/fleet/baseline/services/seaweedfs.yaml` pins; there is no `v`-prefixed
  equivalent — the last `v*` tag is `v3.33`). The pipeline refuses any upstream
  tag that is not a plain `vX.Y.Z` in three places (`render.sh`,
  `verify-pin.sh`, the `pin` job), and the workflow's own trigger is
  `bashy-v*`, so `bashy-4.48-1` would not even fire. Widening the release-tag
  shape is again a `_shared/` change, and it re-renders every existing fork, so
  it wants its own slice.

### Windows legs

No tool declares a Windows matrix leg yet, even where upstream ships one
(gitea publishes windows/amd64 and windows/arm64; the Victoria components
publish windows/amd64). The blocker is the skeleton, not the declarations: its
own steps are bash (`set -euo pipefail`, `. fork/bashy-build/PIN`,
`verify-pin.sh`) and would run under PowerShell on a `windows-*` runner, and
`no-egress-test.sh` is built on `unshare`/`strace` and `sandbox-exec`, with no
Windows equivalent. Adding Windows means a `defaults.run.shell: bash` (or
per-step `shell:`) in `_shared/bashy-build.yml` plus a Windows no-egress
harness — a `_shared/` slice of its own. Windows hosts run the Linux build in
bashy's Linux VM/container meanwhile.

## What the shared pipeline does (every tool)

- **pin**: refuses to run outside `qiangli/<tool>`, refuses an upstream tag
  that is not a plain `vX.Y.Z`, refuses a bashy tag other than
  `bashy-<upstream tag>-N`, and refuses an upstream GitHub release that is a
  draft or pre-release.
- **build** (one leg per `MATRIX` entry): checks out the fork (`fork/`) and the
  upstream tag (`src/`); `verify-pin.sh` fails unless HEAD and the tag both
  equal `UPSTREAM_COMMIT`, the tree is pristine and (if declared)
  `RUST_TOOLCHAIN` matches; installs `APT_PACKAGES` on Linux; runs the
  `setup` steps, then each `PRE_BUILD_GATES` script as `bashy-build/<gate> src`,
  then the `build` steps (which write their outputs to `out/`); then
  `bashy-build/no-egress-test.sh "$EGRESS_TARGET"`, then uploads `out/*`.
- **audit** (only when an `audit` fragment exists): the same checkout + pin
  verification, then the `audit` steps (writing SBOM and other outputs to `out/`).
- **release**: copies `RELEASE_EXTRA` files from `bashy-build/`, chmods
  `RELEASE_EXECUTABLES`, fails if any `RELEASE_ASSETS` file is missing, writes
  `SHA256SUMS`, then `cosign sign-blob --bundle` signs every asset (cosign
  `COSIGN_VERSION`) and self-verifies it against the exact identity. It then
  runs `actions/attest` over `SHA256SUMS` and publishes a GitHub release on the
  fork.

Sigstore identity of a release cut from tag `T`:
`https://github.com/qiangli/<tool>/.github/workflows/bashy-build.yml@refs/tags/T`,
issuer `https://token.actions.githubusercontent.com`. Every action is pinned by
commit sha in the skeleton. Bump an action there once, and every tool gets it.

## The declaration — `builds/<tool>/tool.env`

Shell-sourceable `KEY=VALUE` only. It is shipped verbatim as
`bashy-build/PIN`, so workflow steps and scripts `. bashy-build/PIN` to read it.

| key | meaning |
|---|---|
| `TOOL` | directory name; fork is `FORK_REPO=qiangli/$TOOL` |
| `UPSTREAM_REPO`, `UPSTREAM_SLUG` | upstream URL and `owner/repo` |
| `UPSTREAM_TAG`, `UPSTREAM_COMMIT` | pinned release tag and its full 40-hex commit |
| `KIND` | step-fragment kind under `_shared/kinds/` (`cargo`, `go`), or `custom` (the tool supplies its own fragments) |
| `COSIGN_VERSION` | cosign release used to sign (`v3.1.3`) |
| `MATRIX` | `ASSET=RUNNER ...`; each leg's artifact is named `ASSET` |
| `APT_PACKAGES` | Linux host packages (include the no-egress tools: `strace iproute2 curl`, ...) |
| `PRE_BUILD_GATES` | scripts in `files/` run as `<gate> src` before the build |
| `SHARED_FILES` | plain file names under `_shared/`, copied into `bashy-build/` next to the tool's own `files/`; `*.sh` is made executable. One reviewed copy serves every tool that declares it (`egress-verdict.sh` today). A name that also exists in the tool's `files/` fails the render — no silent shadowing |
| `EGRESS_TARGET` | argument of `no-egress-test.sh` (may be a `${{ }}` expression) |
| `RELEASE_EXTRA` | `SRC=ASSET` copies from `bashy-build/` into the release |
| `RELEASE_ASSETS` | files that must exist before signing |
| `RELEASE_EXECUTABLES` | assets to `chmod +x` |
| `RELEASE_NOTE`, `RELEASE_GATES` | release-notes lines |
| kind keys | `cargo`: `RUST_TOOLCHAIN CARGO_AUDIT_VERSION CARGO_CYCLONEDX_VERSION CARGO_PACKAGE CARGO_BIN CARGO_SBOM_MANIFEST`, optional `files/features.env` (`FEATURES` -> `--no-default-features --features`), required `files/audit-ignore` |
| kind keys | `go`: `GO_TOOLCHAIN GO_PACKAGE GO_LDFLAGS GO_BUILD_TAGS GO_BUILD_FLAGS GO_BUILD_ENV GO_CGO GO_VERSION_ARG GOVULNCHECK_VERSION GO_SBOM_VERSION`, optional `GO_TOOLCHAIN_SECURITY_OVERRIDE`, required `files/vuln-ignore` |

Templates use `@@KEY@@`. A key that is not declared fails the render. That
includes keys a tool's own fragments use, such as openclaw's `NODE_VERSION` and
`PNPM_VERSION`.

### `GO_TOOLCHAIN_SECURITY_OVERRIDE` (the `go` kind)

`GO_TOOLCHAIN` must equal the `go`/`toolchain` directive in the pinned tag's
`go.mod` — the kind's setup step gates that, so a pin bump cannot silently
build on whatever the runner ships. That leaves one real gap: an upstream tag
whose own toolchain **predates a Go stdlib security fix**, with no later
upstream tag yet. Waiving the finding is not an option (reachable
vulnerabilities are never waived; `vuln-ignore` is for unreachable ones), and
bumping `GO_TOOLCHAIN` would be a lie about what upstream builds with.

So the declaration may name a *recorded security override*:

```
GO_TOOLCHAIN=1.27.1                     # what the tag's go.mod says
GO_TOOLCHAIN_SECURITY_OVERRIDE=1.27.2   # what bashy builds with, and why (comment)
```

Both the `build` and `audit` jobs then run `GOTOOLCHAIN=go<override>`, so the
shipped binary and the vulnerability report agree. The gate accepts **exactly**
the declared value and nothing else: it must be `MAJOR.MINOR.PATCH`, stay on
the upstream Go minor, and be a strictly later patch — an override can never
become a language-version change. Choose the newest patch of that minor. It is
shipped in `PIN` (and so in the `<tool>-pin.txt` release asset), the step logs
it, and the declaration's `RELEASE_GATES` names it in the release notes. The
override retires itself the moment an upstream tag carries the fix.

## Conventions every tool must follow (render.sh enforces the first two)

1. `files/otel.env` sets `OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:31415`,
   and every `*ENDPOINT=` in it is loopback. If the tool has no OTel SDK,
   disable it there as well (`OTEL_SDK_DISABLED=true`, exporters `none`).
2. `files/no-egress-test.sh TARGET` is the runtime gate. It must start the
   built artifact in an isolated network (Linux: `unshare` netns + `strace`),
   exercise it over loopback, and **fail** on any non-loopback connection
   attempt. The **harness** is tool-specific — a server needs a readiness probe
   and a CLI needs representative invocations — but the **verdict** is not:
   declare `SHARED_FILES="egress-verdict.sh"`, source it, and call
   `egress_verdict "$log" PORT...`. Copy the closest existing harness: tuwunel
   for a native binary, or openclaw for a Node app with an OTLP mock.

   The verdict is one reviewed decision, so a judgement call lands once:
   **AF_UNIX is local IPC, never egress.** A connect to the glibc nscd or
   systemd-resolved socket is a file on this host, not a packet leaving it;
   counting it as egress is what failed the Linux leg on tuwunel and then on
   all three Victoria forks. A lookup that does leave still appears as the
   AF_INET/AF_INET6 send to port 53 that follows, which the verdict catches.
   openclaw's harness keeps its own POSIX-sh grep verdict (different trace
   flags, different shell).
3. No-tracking knobs go in `files/no-tracking.env` and/or a shipped config
   file. Document each knob in the tool README with its upstream source line.
4. Audit waivers live in a reviewed file in `files/` with one justification
   per entry, such as `audit-ignore` (cargo) or `audit-exceptions` (openclaw).
   Empty means nothing is waived.

## Adding a tool

1. `mkdir builds/<tool>/files`. Write `tool.env` and copy the closest one:
   `tuwunel` for Rust/cargo, or `openclaw` for a custom toolchain.
2. Add `files/otel.env`, `files/no-egress-test.sh` (executable), any
   no-tracking config and the audit-waiver file.
3. If `_shared/kinds/<KIND>` exists, you are done with steps. If it does not,
   write `setup.yml` / `build.yml` / (optional) `audit.yml` next to `tool.env`.
   These are workflow steps at 6-space indent. The build steps must leave the
   release outputs in `out/`. For a toolchain that a second tool will reuse,
   move the fragments to `_shared/kinds/<kind>/` instead. A tool may also
   override ONE fragment of an existing kind and inherit the rest: `gitea` is
   `KIND=go` with its own `build.yml`, because the `bindata` tag embeds a
   vite-built frontend that the generic Go build knows nothing about.
4. `builds/render.sh <tool> && builds/check.sh`. Commit `tool.env`, `files/`,
   the fragments and the generated `fork/`.
5. Operator (not in bashy CI): in a clone of `qiangli/<tool>`, run
   `git switch --orphan bashy-build && cp -R <bashy>/builds/<tool>/fork/. .`,
   commit, push, and tag `bashy-<upstream tag>-1`. Then pin the release in
   yoke binmgr with the identity above.

Bumping a pin means editing `tool.env`, re-running `render.sh`, reviewing the
`fork/` diff, and cutting `bashy-<new tag>-1`. Edit `_shared/` for the
pipeline itself. `check.sh` then shows every fork that has to be re-synced.
