# tailscale — bashy verified build

The outpost runtime image runs tailscaled and its CLI only from a
**bashy-CI-built, keylessly Sigstore-signed build of the official upstream
release tag**. This directory is the declaration for that build. `fork/` is
installed as the `bashy-build` branch of `github.com/qiangli/tailscale`; it
carries no upstream source — the workflow checks out the upstream tag fresh on
every run.

| | |
|---|---|
| Upstream | `https://github.com/tailscale/tailscale` |
| License at the tag | **BSD-3-Clause** (`LICENSE`, verified at `v1.104.1`) |
| Pinned release | `v1.104.1`, the newest stable (annotated tag `7f4efe81…`) → commit `9a522a9786c97eb7910c01ccb7bd66557b04c910` |
| Toolchain | Go `1.27.1` (the tag's `go.mod`), built with the in-minor security override `1.27.2` |
| Trigger | push of tag `bashy-v1.104.1-N` on `bashy-build` (first: `bashy-v1.104.1-1`) |
| Signing identity | `https://github.com/qiangli/tailscale/.github/workflows/bashy-build.yml@refs/tags/bashy-v1.104.1-1` |
| Issuer | `https://token.actions.githubusercontent.com` |

## Layout

```
tool.env                  the declaration (shipped as PIN)
build.yml                 builds tailscaled + tailscale (replaces the go kind's build.yml)
audit.yml                 govulncheck over both packages + module SBOM
files/no-egress-test.sh   runtime gate: an unconfigured tailscaled dials nothing
files/otel.env            OpenTelemetry environment (disabled; local collector only)
files/no-tracking.env     TS_NO_LOGS_NO_SUPPORT + proxy clearing
files/vuln-ignore         govulncheck waivers (empty)
```

## Build configuration, and how it differs from upstream

Upstream's `build_dist.sh` is a plain `go build -trimpath` with
`version.longStamp`/`shortStamp` from `cmd/mkversion`; release binaries are
cgo-free and built with Tailscale's own Go fork. The bashy build:

| | upstream release | bashy build |
|---|---|---|
| binaries | `tailscale`, `tailscaled` in a tarball | `tailscale-linux-<arch>`, `tailscaled-linux-<arch>` |
| toolchain | the `tailscale/go` fork (`go.toolchain.rev`) | stock go1.27.2 (the tag's `go 1.27.1` + security patch) |
| tags | none | `ts_omit_logtail,ts_omit_clientupdate,ts_omit_cloud` |
| ldflags | `longStamp`, `shortStamp` | the same values (`1.104.1-t9a522a978`, `1.104.1`) plus `gitCommitStamp`; `-s -w -buildid=`, `-buildvcs=false` |

The three omitted features are tailscale's own feature tags
(`go run ./cmd/featuretags --list`):

- `logtail` — log upload to `log.tailscale.com`. Compiled out rather than
  merely switched off; `TS_NO_LOGS_NO_SUPPORT=true` in `no-tracking.env` is
  the runtime switch for the same decision, kept as defence in depth.
- `clientupdate` — self-update. bashy updates by re-pinning this build.
- `cloud` — the `169.254.169.254` metadata probe tailscaled makes at startup
  when DMI says it runs on a cloud VM (GitHub's Azure runners included).

The build fails on an ELF interpreter, if `go version -m` lacks any declared
tag, if `--version` does not report the pinned version and commit, or if the
build rewrote `go.mod`/`go.sum`.

## Audit

govulncheck v1.9.0 of `./cmd/tailscaled` and `./cmd/tailscale` with the build
tags, `CGO_ENABLED=0`: under the tag's go1.27.1 eleven stdlib findings are
reachable (GO-2026-6603 … -6617); under go1.27.2 **0 reachable, nothing
waived, no module override**. The SBOM is `cyclonedx-gomod app` of
`./cmd/tailscaled`; the CLI lives in the same module.

## Release assets

`tailscale-linux-{amd64,arm64}`, `tailscaled-linux-{amd64,arm64}`,
`tailscale-<tag>.cdx.json`, `tailscale-<tag>.govulncheck.json`,
`tailscale-pin.txt`, `tailscale-otel.env`, `tailscale-no-tracking.env`,
`vuln-ignore`, each with a `.sigstore.json` bundle, plus `SHA256SUMS`.

## Operator steps (not in bashy CI)

```sh
# in a clone of github.com/qiangli/tailscale
git switch --orphan bashy-build
cp -R <bashy>/builds/tailscale/fork/. .
git add .github/workflows/bashy-build.yml bashy-build
git commit -m "bashy-build: tailscale v1.104.1 verified build"
git push -u origin bashy-build
git tag bashy-v1.104.1-1 && git push origin bashy-v1.104.1-1
```

Then pin the release in binmgr against the signing identity and issuer above.
