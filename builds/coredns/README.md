# coredns — bashy verified build

The outpost runtime image serves k3s its cluster DNS only from a
**bashy-CI-built, keylessly Sigstore-signed build of the official upstream
release tag**. This directory is the declaration for that build. `fork/` is
installed as the `bashy-build` branch of `github.com/qiangli/coredns`; it
carries no upstream source — the workflow checks out the upstream tag fresh on
every run.

| | |
|---|---|
| Upstream | `https://github.com/coredns/coredns` |
| License at the tag | **Apache-2.0** (`LICENSE`, verified at `v1.14.7`) |
| Pinned release | `v1.14.7` (lightweight tag) → commit `427fc80ed9ca47f354585eb30a3f1332950856c4` |
| Why this tag | the newest stable CoreDNS release (k3s's own manifest names 1.14.3) |
| Toolchain | Go `1.25.0` (the tag's `go.mod`), built with the EOL-minor security override `1.26.9` |
| Trigger | push of tag `bashy-v1.14.7-N` on `bashy-build` (first: `bashy-v1.14.7-1`) |
| Signing identity | `https://github.com/qiangli/coredns/.github/workflows/bashy-build.yml@refs/tags/bashy-v1.14.7-1` |
| Issuer | `https://token.actions.githubusercontent.com` |

## Layout

```
tool.env                  the declaration (shipped as PIN)
files/no-egress-test.sh   runtime gate: a self-answering Corefile, one real DNS query
files/otel.env            OpenTelemetry environment (disabled; local collector only)
files/no-tracking.env     proxy clearing
files/vuln-ignore         govulncheck waivers (empty)
```

Build, audit and setup are the `go` kind's, unchanged.

## Build configuration

Upstream's `make coredns`: `CGO_ENABLED=0 go build -tags grpcnotrace
-ldflags "-s -w -X …coremain.GitCommit=$(git describe)"` of the root package
with the default `plugin.cfg` (the generated `zplugin.go`/`zdirectives.go` are
committed at the tag). The bashy build is the same package and tags; the
differences are `-trimpath -buildid=`, `GitCommit` = the pinned commit, and
the toolchain (go1.26.9, not upstream's `.go-version` go1.26.6). Static by
construction (no cgo).

## Audit

govulncheck v1.9.0 of `.` under go1.26.9 with `-tags grpcnotrace`: at the
tag's `go.mod` seven findings are reachable — grpc v1.83.0 (GO-2026-6348,
GO-2026-6443, fixed in v1.83.2) and x/net v0.57.0 (GO-2026-6603, -6610,
-6611, -6612, -6617, fixed in v0.60.0). `GO_SECURITY_OVERRIDES` raises grpc
and x/net plus exactly the seven `golang.org/x` require lines MVS moves with
them; each stays within its major, none is added, and the `go` directive rises
1.25.0 → 1.26.0 (x/net v0.60.0 declares it), within the effective go1.26.9.
govulncheck is then **0 reachable, nothing waived**. Each entry retires when
an upstream tag carries the bump.

## How the runtime image serves it to k3s

This pipeline produces static binaries only; the image is built elsewhere.
The bashy k3s build embeds no `coredns.yaml`, so k3s never deploys an upstream
CoreDNS image. The runtime image supplies cluster DNS itself:

1. Wrap `coredns-linux-<arch>` in a `FROM scratch` OCI image (the binary at
   `/coredns`, `ENTRYPOINT ["/coredns"]`), tagged with a bashy-owned name,
   e.g. `bashy.local/coredns:v1.14.7-bashy1`.
2. Deliver the image without a registry pull: write it as an OCI/docker
   archive into k3s's airgap directory
   (`/var/lib/rancher/k3s/agent/images/*.tar`), which k3s imports into
   containerd at agent start. A local registry declared in
   `/etc/rancher/k3s/registries.yaml` is the alternative when images must be
   updated without restarting the agent.
3. Deploy it with the image's own manifest in
   `/var/lib/rancher/k3s/server/manifests/` (k3s's coredns.yaml shape: the
   `kube-dns` Service at the `--cluster-dns` address, a Corefile using the
   `kubernetes` plugin), with `imagePullPolicy: Never`.

## Release assets

`coredns-linux-{amd64,arm64}`, `coredns-<tag>.cdx.json`,
`coredns-<tag>.govulncheck.json`, `coredns-pin.txt`, `coredns-otel.env`,
`coredns-no-tracking.env`, `vuln-ignore`, each with a `.sigstore.json`
bundle, plus `SHA256SUMS`.

## Operator steps (not in bashy CI)

```sh
# in a clone of github.com/qiangli/coredns
git switch --orphan bashy-build
cp -R <bashy>/builds/coredns/fork/. .
git add .github/workflows/bashy-build.yml bashy-build
git commit -m "bashy-build: coredns v1.14.7 verified build"
git push -u origin bashy-build
git tag bashy-v1.14.7-1 && git push origin bashy-v1.14.7-1
```

Then pin the release in binmgr against the signing identity and issuer above.
