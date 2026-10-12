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
  check.sh                  regenerate-and-diff gate (+ bash -n, shellcheck, actionlint)
  _shared/
    bashy-build.yml         workflow skeleton: pin -> build matrix -> [audit] -> release
    verify-pin.sh           provenance gate, shipped into every fork
    kinds/cargo/            setup.yml / build.yml / audit.yml steps for Rust tools
  <tool>/
    tool.env                THE declaration (shipped verbatim as bashy-build/PIN)
    files/                  tool-specific files, copied into bashy-build/
    setup.yml build.yml audit.yml   optional step fragments; each overrides the kind's
    README.md               knob tables, upstream evidence, binmgr pin
    fork/                   GENERATED — never edit by hand
```

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
| `KIND` | step-fragment kind under `_shared/kinds/` (`cargo`), or `custom` (the tool supplies its own fragments) |
| `COSIGN_VERSION` | cosign release used to sign (`v3.1.3`) |
| `MATRIX` | `ASSET=RUNNER ...`; each leg's artifact is named `ASSET` |
| `APT_PACKAGES` | Linux host packages (include the no-egress tools: `strace iproute2 curl`, ...) |
| `PRE_BUILD_GATES` | scripts in `files/` run as `<gate> src` before the build |
| `EGRESS_TARGET` | argument of `no-egress-test.sh` (may be a `${{ }}` expression) |
| `RELEASE_EXTRA` | `SRC=ASSET` copies from `bashy-build/` into the release |
| `RELEASE_ASSETS` | files that must exist before signing |
| `RELEASE_EXECUTABLES` | assets to `chmod +x` |
| `RELEASE_NOTE`, `RELEASE_GATES` | release-notes lines |
| kind keys | `cargo`: `RUST_TOOLCHAIN CARGO_AUDIT_VERSION CARGO_CYCLONEDX_VERSION CARGO_PACKAGE CARGO_BIN CARGO_SBOM_MANIFEST`, optional `files/features.env` (`FEATURES` -> `--no-default-features --features`), required `files/audit-ignore` |

Templates use `@@KEY@@`. A key that is not declared fails the render. That
includes keys a tool's own fragments use, such as openclaw's `NODE_VERSION` and
`PNPM_VERSION`.

## Conventions every tool must follow (render.sh enforces the first two)

1. `files/otel.env` sets `OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:31415`,
   and every `*ENDPOINT=` in it is loopback. If the tool has no OTel SDK,
   disable it there as well (`OTEL_SDK_DISABLED=true`, exporters `none`).
2. `files/no-egress-test.sh TARGET` is the runtime gate. It must start the
   built artifact in an isolated network (Linux: `unshare` netns + `strace`),
   exercise it over loopback, and **fail** on any non-loopback connection
   attempt. Its harness is tool-specific: a server needs a readiness probe and
   a CLI needs representative invocations. Copy the closest existing one:
   tuwunel for a native binary, or openclaw for a Node app with an OTLP mock.
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
   move the fragments to `_shared/kinds/<kind>/` instead. The upcoming CLI
   migration will probably add `go`.
4. `builds/render.sh <tool> && builds/check.sh`. Commit `tool.env`, `files/`,
   the fragments and the generated `fork/`.
5. Operator (not in bashy CI): in a clone of `qiangli/<tool>`, run
   `git switch --orphan bashy-build && cp -R <bashy>/builds/<tool>/fork/. .`,
   commit, push, and tag `bashy-<upstream tag>-1`. Then pin the release in
   yoke binmgr with the identity above.

Bumping a pin means editing `tool.env`, re-running `render.sh`, reviewing the
`fork/` diff, and cutting `bashy-<new tag>-1`. Edit `_shared/` for the
pipeline itself. `check.sh` then shows every fork that has to be re-synced.
