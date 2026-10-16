# metrics-server — bashy verified build

The outpost runtime image runs metrics-server only from a **bashy-CI-built,
keylessly Sigstore-signed build of the official upstream release tag**. This
directory is the declaration for that build. `fork/` is installed as the
`bashy-build` branch of `github.com/qiangli/metrics-server`; it carries no
upstream source — the workflow checks out the upstream tag fresh on every run.

| | |
|---|---|
| Upstream | `https://github.com/kubernetes-sigs/metrics-server` |
| License at the tag | **Apache-2.0** (`LICENSE`, verified at `v0.9.0`) |
| Pinned release | `v0.9.0` (lightweight tag) → commit `2a7c4b2c7d46552ff47f4aeaa3a735c582587ecd` |
| Toolchain | Go `1.26.4` (the tag's `go.mod`), built with the in-minor security override `1.26.9` |
| Trigger | push of tag `bashy-v0.9.0-N` on `bashy-build` (first: `bashy-v0.9.0-1`) |
| Signing identity | `https://github.com/qiangli/metrics-server/.github/workflows/bashy-build.yml@refs/tags/bashy-v0.9.0-1` |
| Issuer | `https://token.actions.githubusercontent.com` |

## Layout

```
tool.env                  the declaration (shipped as PIN)
files/no-egress-test.sh   runtime gate: dials only its loopback apiserver
files/otel.env            OpenTelemetry environment (disabled; local collector only)
files/no-tracking.env     proxy clearing
files/vuln-ignore         govulncheck waivers (empty)
```

Setup, build and audit are the `go` kind's, unchanged; `GO_SECURITY_OVERRIDES`
is the only non-default knob (§Audit).

## Build configuration, and how it differs from upstream

Upstream's Makefile builds `CGO_ENABLED=0 go build -mod=readonly -trimpath
-ldflags "-w $(VERSION_LDFLAGS)" sigs.k8s.io/metrics-server/cmd/metrics-server`
and ships it in a container image. The bashy build is the same package, cgo off,
with these differences:

| | upstream release | bashy build |
|---|---|---|
| artifact | container image | the static binary `metrics-server-linux-{amd64,arm64}` (the image lays it out itself) |
| toolchain | the tag's go1.26.4 | go1.26.9 (stdlib security patches) |
| modules | the tag's go.mod | 19 require lines raised within their major (`GO_SECURITY_OVERRIDES`) |
| ldflags | `gitVersion`, `gitCommit`, `buildDate` | the same from the pin, no `buildDate` |

## Audit

govulncheck v1.9.0 of `./cmd/metrics-server` under go1.26.9 at the pinned
commit: every stdlib finding is settled by the toolchain patch. These module
findings are reachable at the tag's own `go.mod`:

| module | at the tag | fixed in | findings |
|---|---|---|---|
| `golang.org/x/net` | v0.56.0 | v0.60.0 | GO-2026-6603, -6610, -6611, -6612, -6617 |
| `google.golang.org/grpc` | v1.81.1 | v1.83.1 | GO-2026-6061, -6348 |
| `go.opentelemetry.io/otel/{sdk,exporters/otlp/otlptrace,…/otlptracegrpc}` | v1.44.0 | v1.45.0 | GO-2026-6505 |
| `golang.org/x/text` | v0.38.0 | v0.39.0 (x/net v0.60.0 takes it to v0.42.0) | GO-2026-5970 |

`GO_SECURITY_OVERRIDES` raises x/net, grpc and otlptracegrpc, and declares
the 16 require lines MVS moves with them, so the module gate admits exactly
that set: 19 require lines, each within its major, none added, the `go`
directive unchanged, and the tree builds with `-mod=readonly` and no
`go mod tidy`. govulncheck is then clean (0 reachable); `vuln-ignore` stays
empty. v0.8.1 was not fixable this way (55 moves including
`k8s.io/{api,apimachinery,client-go}` across minors, and one added require);
v0.9.0 is what made the pin auditable.

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
git commit -m "bashy-build: metrics-server v0.9.0 verified build"
git push -u origin bashy-build
git tag bashy-v0.9.0-1 && git push origin bashy-v0.9.0-1
```

Then pin the release in binmgr against the signing identity and issuer above.
