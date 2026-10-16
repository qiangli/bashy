# cni-plugins — bashy verified build

The outpost runtime image's k3s agent runs its CNI plugins only from a
**bashy-CI-built, keylessly Sigstore-signed build of the official upstream
release tag**. This directory is the declaration for that build. `fork/` is
installed as the `bashy-build` branch of `github.com/qiangli/cni-plugins`; it
carries no upstream source — the workflow checks out the upstream tag fresh on
every run.

| | |
|---|---|
| Upstream | `https://github.com/containernetworking/plugins` |
| License at the tag | **Apache-2.0** (`LICENSE`, verified at `v1.5.1`) |
| Pinned release | `v1.5.1` (lightweight tag) → commit `c29dc79f96cd50452a247a4591443d2aac033429` |
| Toolchain | Go `1.20` (the tag's `go.mod`), built with the EOL-minor security override `1.26.9` |
| Trigger | push of tag `bashy-v1.5.1-N` on `bashy-build` (first: `bashy-v1.5.1-1`) |
| Signing identity | `https://github.com/qiangli/cni-plugins/.github/workflows/bashy-build.yml@refs/tags/bashy-v1.5.1-1` |
| Issuer | `https://token.actions.githubusercontent.com` |

## Layout

```
tool.env                  the declaration (shipped as PIN)
build.yml                 builds every CNI_PLUGINS entry (replaces the go kind's build.yml)
audit.yml                 govulncheck over the shipped plugins + module SBOM
files/no-egress-test.sh   runtime gate: the plugins make no outbound connection
files/otel.env            OpenTelemetry environment (disabled; local collector only)
files/no-tracking.env     proxy clearing
files/vuln-ignore         govulncheck waivers (empty)
```

## Build configuration, and how it differs from upstream

Upstream `scripts/release.sh` runs `build_linux.sh` with `CGO_ENABLED=0`,
`-mod=vendor` and `-ldflags '-extldflags -static -X …BuildVersion=<tag>'` over
every plugin directory. The bashy build is the same per plugin, with these
differences:

| | upstream release | bashy build |
|---|---|---|
| plugins | all of `plugins/{main,ipam,meta}` | `CNI_PLUGINS`: bridge, host-local, loopback, portmap, bandwidth, firewall — what k3s's flannel conflist and containerd's CRI loopback call |
| toolchain | the release host's | go1.26.9 (go1.20 is end-of-life); `-trimpath`, `-buildid=`, no `-extldflags` (no cgo, nothing external to link) |
| assets | one `.tgz` per arch | one static binary per plugin, `cni-<name>-linux-<arch>` |

Pure Go, no cgo: the build fails on a binary with an ELF interpreter, on a
plugin whose about string does not name the pinned tag, or if the build
changed `go.mod`/`go.sum`/`vendor/`. Nothing is fetched at build time — every
module is in the tag's `vendor/`.

go1.20 predates `GOTOOLCHAIN`, so the toolchain `setup-go` installs from
`go.mod` would ignore the override. `build.yml` and `audit.yml` therefore
install the override release explicitly and assert `go env GOVERSION` equals
the toolchain the setup gate selected.

## Audit

govulncheck in source mode over the six shipped plugin packages, under the
build's toolchain: **0 reachable findings, nothing waived, no module override**
(measured at the pinned commit). The SBOM is `cyclonedx-gomod mod` of the one
module all plugins share, a superset of each plugin's components.

## Release assets

`cni-{bridge,host-local,loopback,portmap,bandwidth,firewall}-linux-{amd64,arm64}`,
`cni-plugins-<tag>.cdx.json`, `cni-plugins-<tag>.govulncheck.json`,
`cni-plugins-pin.txt`, `cni-plugins-otel.env`, `cni-plugins-no-tracking.env`,
`vuln-ignore`, each with a `.sigstore.json` bundle, plus `SHA256SUMS`.

## Operator steps (not in bashy CI)

```sh
# in a clone of github.com/qiangli/cni-plugins
git switch --orphan bashy-build
cp -R <bashy>/builds/cni-plugins/fork/. .
git add .github/workflows/bashy-build.yml bashy-build
git commit -m "bashy-build: cni-plugins v1.5.1 verified build"
git push -u origin bashy-build
git tag bashy-v1.5.1-1 && git push origin bashy-v1.5.1-1
```

Then pin the release in binmgr against the signing identity and issuer above.
