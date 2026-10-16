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
| `k3s` | `./cmd/server`, cgo, static (`-extldflags -static`) in an alpine (musl) image | **same package and tags minus `libsqlite3`**, static against musl via Ubuntu's `musl-gcc` |
| sqlite | system `libsqlite3` (tag) | go-sqlite3's bundled amalgamation (public domain) |
| `containerd-shim-runc-v2` | from a containerd clone, cgo static | from the containerd module k3s's `go.mod` pins (`k3s-io/containerd/v2`), `CGO_ENABLED=0` as containerd ships it |
| `runc` | built with `seccomp` (links libseccomp, LGPL-2.1) | **not built here** — a separate pipeline (`builds/runc`) |
| CNI plugins | `rancher/plugins` + flannel cni-plugin clones | **not built here** — separate pipelines |
| embedded manifests | all of `manifests/` | only `ccm.yaml rolebindings.yaml runtimes.yaml` (`K3S_MANIFESTS`): the ones that reference no image. coredns, local-storage, metrics-server and traefik are not in the binary |
| self-extracting data dir | `cmd/k3s` wrapper embedding the above | **none**: the release is the bare multicall binary; the image lays out `containerd`/`kubectl`/`crictl`/`ctr` as links to it |
| ldflags | `VERSIONFLAGS` incl. build date | the same stamps from the pin; no build date |

The build fails if a binary has an ELF interpreter (not static), if
`k3s --version` does not report the pinned tag, or if the build rewrote
`go.mod`/`go.sum`.

The kernel UAPI headers the btrfs snapshotter compiles against come from
`linux-libc-dev` (GPL-2.0 WITH Linux-syscall-note): headers only, nothing of
them is linked.

## Runtime notes for the image that consumes this

- k3s calls `iptables`/`ip6tables`(-save/-restore), `ipset`, `nft`, `mount`,
  `modprobe` at runtime (agent side: kube-proxy, flannel, network policy,
  kubelet). None of these are in this release; the server with
  `--disable-agent` needs none of them.
- The agent pulls the pause image (`--pause-image`) and anything a manifest
  names; serve those from bashy-built images.

## Audit

`audit.yml` runs govulncheck in source mode over `./cmd/server` and the shim
package together, with the k3s build tags and cgo on; the SBOM is
`cyclonedx-gomod app` of `./cmd/server`, whose module graph contains the shim's
module. Reachable, unwaived findings fail the release; `files/vuln-ignore` is
empty. A finding the toolchain override does not settle is fixed through
`GO_SECURITY_OVERRIDES` (builds/README.md), never waived.

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
