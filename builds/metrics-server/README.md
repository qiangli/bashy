# metrics-server — bashy verified build

The outpost runtime image runs metrics-server only from a **bashy-CI-built,
keylessly Sigstore-signed build of the official upstream release tag**. This
directory is the declaration for that build. `fork/` is installed as the
`bashy-build` branch of `github.com/qiangli/metrics-server`; it carries no
upstream source — the workflow checks out the upstream tag fresh on every run.

| | |
|---|---|
| Upstream | `https://github.com/kubernetes-sigs/metrics-server` |
| License at the tag | **Apache-2.0** (`LICENSE`, verified at `v0.8.1`) |
| Pinned release | `v0.8.1` (annotated tag `1be06a21…`) → commit `9ce65a47f9262e19e482c259ace815f01135a898` |
| Toolchain | Go `1.24.12` (the tag's `go.mod`), built with the EOL-minor security override `1.26.9` |
| Trigger | push of tag `bashy-v0.8.1-N` on `bashy-build` (first: `bashy-v0.8.1-1`) |
| Signing identity | `https://github.com/qiangli/metrics-server/.github/workflows/bashy-build.yml@refs/tags/bashy-v0.8.1-1` |
| Issuer | `https://token.actions.githubusercontent.com` |

## Layout

```
tool.env                  the declaration (shipped as PIN)
files/no-egress-test.sh   runtime gate: dials only its loopback apiserver
files/otel.env            OpenTelemetry environment (disabled; local collector only)
files/no-tracking.env     proxy clearing
files/vuln-ignore         govulncheck waivers (empty)
```

Setup, build and audit are the `go` kind's, unchanged.

## Build configuration, and how it differs from upstream

Upstream's Makefile builds `CGO_ENABLED=0 go build -mod=readonly -trimpath
-ldflags "-w $(VERSION_LDFLAGS)" sigs.k8s.io/metrics-server/cmd/metrics-server`
and ships it in a container image. The bashy build is the same package, cgo off,
with these differences:

| | upstream release | bashy build |
|---|---|---|
| artifact | container image | the static binary `metrics-server-linux-{amd64,arm64}` (the image lays it out itself) |
| toolchain | the tag's go1.24.12 | go1.26.9 (go1.24 is end-of-life) |
| ldflags | `gitVersion`, `gitCommit`, `buildDate` | the same from the pin, no `buildDate` |

## Audit — release blocked at this pin

govulncheck of `./cmd/metrics-server` under go1.26.9, at the pinned commit:
every stdlib finding is settled; these **module** findings stay reachable:

| module | at the tag | fixed in | findings |
|---|---|---|---|
| `github.com/prometheus/prometheus` | v0.304.2 | v0.311.3 | GO-2026-5264, -5381, -5662, -5710 (module-wide OSVs, so any import counts as reachable) |
| `golang.org/x/net` | v0.40.0 | v0.60.0 | 7 |
| `go.opentelemetry.io/otel/{sdk,exporters/otlp/otlptrace,…/otlptracegrpc}` | v1.35.0 | v1.45.0 | GO-2026-4394, -6505 |
| `google.golang.org/grpc` | v1.72.0 | v1.83.1 | GO-2026-6061, -6348 |
| `golang.org/x/text` | v0.25.0 | v0.42.0 (x/net's floor) | GO-2026-5970 |

`GO_SECURITY_OVERRIDES` cannot express that fix set. MVS changes 56 require
lines: 55 move, including `k8s.io/{api,apimachinery,client-go}` to v0.35.3
against an apiserver at v0.33.7, and one is **added**
(`github.com/cenkalti/backoff/v5`, which otlptracegrpc v1.45.0 needs). The
graph then also needs `go mod tidy` before it builds.
The module gate refuses both, by design. Waiving reachable findings is not an
option either. So `GO_SECURITY_OVERRIDES` is empty, and the audit job fails
at this pin until the pin moves to an upstream tag that carries the fixes
(`v0.9.0` exists; not evaluated here).

## Release assets

`metrics-server-linux-{amd64,arm64}`, `metrics-server-<tag>.cdx.json`,
`metrics-server-<tag>.govulncheck.json`, `metrics-server-pin.txt`,
`metrics-server-otel.env`, `metrics-server-no-tracking.env`, `vuln-ignore`,
each with a `.sigstore.json` bundle, plus `SHA256SUMS`.

## Operator steps (not in bashy CI)

```sh
# in a clone of github.com/qiangli/metrics-server
git switch --orphan bashy-build
cp -R <bashy>/builds/metrics-server/fork/. .
git add .github/workflows/bashy-build.yml bashy-build
git commit -m "bashy-build: metrics-server v0.8.1 verified build"
git push -u origin bashy-build
git tag bashy-v0.8.1-1 && git push origin bashy-v0.8.1-1
```

Then pin the release in binmgr against the signing identity and issuer above.
