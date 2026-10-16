# k3s — bashy verified build

The outpost runtime image runs k3s (control plane and agent) only from a
**bashy-CI-built, keylessly Sigstore-signed build of the official upstream
release tag**. This directory is the declaration for that build. `fork/` is
installed as the `bashy-build` branch of `github.com/qiangli/k3s`; it carries
no upstream source — the workflow checks out the upstream tag fresh on every
run.

| | |
|---|---|
| Upstream | `https://github.com/k3s-io/k3s` |
| License at the tag | **Apache-2.0** (`LICENSE`, verified at `v1.36.1+k3s1`) |
| Pinned release | `v1.36.1+k3s1` (lightweight tag) → commit `a9663261a7ff40522542485a6b2f81916b6d72f9` |
| Why this tag | the release the outpost runtime image downloads today (`K3S_VERSION`) |
| Tag form | `vX.Y.Z+k3sN` (declared in `tool.env`; `builds/render.sh` maps it) |
| Toolchain | Go `1.26.2` (the tag's `go.mod`), built with the in-minor security override `1.26.9` |
| Trigger | push of tag `bashy-v1.36.1+k3s1-N` on `bashy-build` (first: `bashy-v1.36.1+k3s1-1`) |
| Signing identity | `https://github.com/qiangli/k3s/.github/workflows/bashy-build.yml@refs/tags/bashy-v1.36.1+k3s1-1` |
| Issuer | `https://token.actions.githubusercontent.com` |

## Layout

```
tool.env                  the declaration (shipped as PIN)
build.yml                 k3s's own build steps (replaces the go kind's build.yml)
audit.yml                 govulncheck over BOTH shipped packages + SBOM
files/cgo-free-overlay.sh the two-file go build -overlay that lets k3s build CGO_ENABLED=0
files/no-egress-test.sh   runtime gate: a bare control plane makes no outbound connection
files/otel.env            OpenTelemetry environment (disabled; local collector only)
files/no-tracking.env     proxy clearing
files/vuln-ignore         govulncheck waivers (empty)
```

The setup steps (toolchain gate, recorded overrides) are the `go` kind's,
unchanged.

## Build configuration, and how it differs from upstream

Upstream `scripts/download` + `scripts/build` + `scripts/package-cli`:

| | upstream release | bashy build |
|---|---|---|
| network fetches at build time | k3s-root tarball (busybox, iptables, conntrack, … — GPL userland), runc clone, containerd clone, Helm charts from k3s.io | **none** beyond the Go modules `go.sum` pins |
| `k3s` | `./cmd/server`, cgo, static (`-extldflags -static`) in an alpine (musl) image | **same package, `CGO_ENABLED=0`**: upstream tags minus the cgo-only `libsqlite3`/`sqlite_omit_load_extension`/`netcgo`, plus containerd's `no_btrfs`; static by construction, no C toolchain |
| sqlite / datastore | `mattn/go-sqlite3` (cgo) against system `libsqlite3`; sqlite is the default datastore | **not linked**: kine's `!cgo` driver compiles sqlite out. The datastore is **embedded etcd** — the image must run `k3s server --cluster-init` |
| btrfs snapshotter | cgo, linked | **not linked** (`no_btrfs`); overlayfs/native/etc. remain |
| `containerd-shim-runc-v2` | from a containerd clone, cgo static | from the containerd module k3s's `go.mod` pins (`k3s-io/containerd/v2`), `CGO_ENABLED=0` as containerd ships it |
| `runc` | built with `seccomp` (links libseccomp, LGPL-2.1) | **not built here** — a separate pipeline (`builds/runc`) |
| CNI plugins | `rancher/plugins` + flannel cni-plugin clones | **not built here** — separate pipelines |
| embedded manifests | all of `manifests/` | only `ccm.yaml rolebindings.yaml runtimes.yaml` (`K3S_MANIFESTS`): the ones that reference no image. coredns, local-storage, metrics-server and traefik are not in the binary |
| self-extracting data dir | `cmd/k3s` wrapper embedding the above | **none**: the release is the bare multicall binary; the image lays out `containerd`/`kubectl`/`crictl`/`ctr` as links to it |
| ldflags | `VERSIONFLAGS` incl. build date | the same stamps from the pin; no build date |

The build fails if a binary has an ELF interpreter (not static), if
`go version -m` shows `CGO_ENABLED=1` or `mattn/go-sqlite3`, if
`k3s --version` does not report the pinned tag, or if the build rewrote
`go.mod`/`go.sum`.

### Why CGO_ENABLED=0, and the overlay

bashy uses pure-Go sqlite only and never `mattn/go-sqlite3`. kine (k3s's
datastore shim) already has a `!cgo` path that compiles its sqlite driver out;
it cannot take `modernc.org/sqlite` without patching (it registers the
`sqlite3` driver name `mattn/go-sqlite3` provides), so the datastore is
embedded etcd instead.

Tags alone do not get k3s to build without cgo. Two upstream files import a
cgo-only package unconditionally, which leaves an import with no buildable
files. `files/cgo-free-overlay.sh` writes replacements for exactly those two
files and a `go build -overlay` map. It does not edit the checkout, so the
`go.mod`/`go.sum` gate still applies, and each replacement is checked to be the
intended edit, so a tag that moves either file fails the build:

| upstream file | replacement |
|---|---|
| `pkg/containerd/builtins_linux.go` | the same file minus the one `plugins/snapshots/btrfs/plugin` import (`no_btrfs` excludes that package's every file) |
| `pkg/proctitle/proctile.go` | a no-op `SetProcTitle`, like upstream's own windows file (`erikdubbelboer/gspt` is cgo-only; only the `ps` process title is lost) |

The audit job applies the same two files to its own checkout, because
govulncheck ignores `-overlay`, so the audited source and tags match the
shipped ones.

**runc stays the one cgo build** in the runtime image: its `nsenter` is C and
runs before the Go runtime starts. It gets its own pipeline later
(`builds/runc`), not here.

## Runtime notes for the image that consumes this

- Start the server with `--cluster-init` (embedded etcd); with no sqlite in
  the binary, a server without it, and with no `--datastore-endpoint`, does
  not start.
- k3s calls `iptables`/`ip6tables`(-save/-restore), `ipset`, `nft`, `mount`,
  `modprobe` at runtime (agent side: kube-proxy, flannel, network policy,
  kubelet). None of these are in this release; the server with
  `--disable-agent` needs none of them.
- The agent pulls the pause image (`--pause-image`) and anything a manifest
  names; serve those from bashy-built images.

## Audit

`audit.yml` runs govulncheck in source mode over `./cmd/server` and the shim
package together, with the k3s build tags, `CGO_ENABLED=0` and the overlay files applied; the SBOM is
`cyclonedx-gomod app` of `./cmd/server`, whose module graph contains the shim's
module. Reachable, unwaived findings fail the release; `files/vuln-ignore` is
empty. A finding the toolchain override does not settle is fixed through
`GO_SECURITY_OVERRIDES` (builds/README.md), never waived.

### Release blocked at this pin

govulncheck v1.9.0 under go1.26.9 (stdlib settled) reports 24 reachable
module findings at the tag's `go.mod`. `GO_SECURITY_OVERRIDES` can settle 9
of them: raising pion/dtls/v3 v3.1.4, quic-go/webtransport-go v0.11.1,
otlptracegrpc and otel/sdk v1.45.0 and x/text v0.41.0 moves 24 require lines
in total, each within its major, none added, the `go` directive unchanged,
and the tree still builds:

```
github.com/go-logr/logr@v1.4.4
github.com/grpc-ecosystem/grpc-gateway/v2@v2.29.0
github.com/pion/dtls/v3@v3.1.4
github.com/quic-go/quic-go@v0.60.0
github.com/quic-go/webtransport-go@v0.11.1
go.opentelemetry.io/otel@v1.45.0
go.opentelemetry.io/otel/exporters/otlp/otlptrace@v1.45.0
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@v1.45.0
go.opentelemetry.io/otel/metric@v1.45.0
go.opentelemetry.io/otel/sdk@v1.45.0
go.opentelemetry.io/otel/trace@v1.45.0
go.opentelemetry.io/proto/otlp@v1.11.0
golang.org/x/crypto@v0.51.0
golang.org/x/mod@v0.38.0
golang.org/x/net@v0.57.0
golang.org/x/oauth2@v0.36.0
golang.org/x/sync@v0.22.0
golang.org/x/sys@v0.47.0
golang.org/x/telemetry@v0.0.0-20260708182218-49f421fb7959
golang.org/x/text@v0.41.0
golang.org/x/tools@v0.48.0
google.golang.org/genproto/googleapis/api@v0.0.0-20260803160001-6ac0973c030d
google.golang.org/genproto/googleapis/rpc@v0.0.0-20260803160001-6ac0973c030d
google.golang.org/grpc@v1.83.0
```

The other 15 stay reachable, and the gate cannot express their fix. Five
modules are pinned by k3s's own `replace` directives, so `go get` moves only
an inert require line and the gate refuses a replace edit by design:

| module (replaced to) | fixed in | findings |
|---|---|---|
| `golang.org/x/net` v0.49.0 | v0.60.0 | GO-2026-4918, -5026, -6603, -6610, -6611, -6612, -6617 |
| `google.golang.org/grpc` v1.79.3 | v1.83.1 | GO-2026-6061, -6348, -6443 |
| `github.com/cilium/ebpf` v0.12.3 | v0.22.0 (crosses a minor k3s pins) | GO-2026-6238 |
| `github.com/docker/docker` v25.0.15-…d2dbc0547253 | no fix | GO-2026-4883, -4887 |
| `golang.org/x/crypto` v0.47.0 | no fix (openpgp) | GO-2026-5932 |

and `github.com/libp2p/go-libp2p-kad-dht` v0.39.0 has no fixed version
(GO-2024-3218). So `GO_SECURITY_OVERRIDES` stays empty and the audit job
fails at this pin until an upstream k3s tag carries the replace bumps and
the unfixed modules are gone or fixed upstream.

## Release assets

`k3s-linux-{amd64,arm64}`, `containerd-shim-runc-v2-linux-{amd64,arm64}`,
`k3s-<tag>.cdx.json`, `k3s-<tag>.govulncheck.json`, `k3s-pin.txt`,
`k3s-otel.env`, `k3s-no-tracking.env`, `vuln-ignore`, each with a
`.sigstore.json` bundle, plus `SHA256SUMS`.

## Operator steps (not in bashy CI)

```sh
# in a clone of github.com/qiangli/k3s
git switch --orphan bashy-build
cp -R <bashy>/builds/k3s/fork/. .
git add .github/workflows/bashy-build.yml bashy-build
git commit -m "bashy-build: k3s v1.36.1+k3s1 verified build"
git push -u origin bashy-build
git tag bashy-v1.36.1+k3s1-1 && git push origin bashy-v1.36.1+k3s1-1
```

Then pin the release in binmgr against the signing identity and issuer above.
