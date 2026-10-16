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
| License at the tag | **Apache-2.0** (`LICENSE`, verified at `v1.37.1+k3s1`) |
| Pinned release | `v1.37.1+k3s1` (lightweight tag) → commit `356c025475bbefaa09cf0d7e48e8e2adbbde57db` |
| Why this tag | the newest stable k3s release, and the one whose bashy build audits clean with no waiver (operator decision 2026-10-16, "k3s option 1, no waivers"); the regression map below is everything that took |
| Tag form | `vX.Y.Z+k3sN` (declared in `tool.env`; `builds/render.sh` maps it) |
| Toolchain | Go `1.26.8` (the tag's `go.mod`), built with the in-minor security override `1.26.9` |
| Trigger | push of tag `bashy-v1.37.1+k3s1-N` on `bashy-build` (first: `bashy-v1.37.1+k3s1-1`) |
| Signing identity | `https://github.com/qiangli/k3s/.github/workflows/bashy-build.yml@refs/tags/bashy-v1.37.1+k3s1-1` |
| Issuer | `https://token.actions.githubusercontent.com` |

The outpost runtime image (`internal/agent/runtime/image/Dockerfile`) still
downloads the upstream `v1.36.1+k3s1` release; moving it to this build is the
image's change, not this one.

## Layout

```
tool.env                  the declaration (shipped as PIN)
build.yml                 k3s's own build steps (replaces the go kind's build.yml)
audit.yml                 govulncheck over BOTH shipped packages + SBOM
files/k3s-overlay.sh      the sha256-pinned go build -overlay (cgo-free + the no-fix advisory removals)
files/no-egress-test.sh   runtime gate: a bare control plane makes no outbound connection
files/otel.env            OpenTelemetry environment (disabled; local collector only)
files/no-tracking.env     proxy clearing
files/vuln-ignore         govulncheck waivers (empty)
```

The setup steps (toolchain gate, `GO_SECURITY_OVERRIDES` with its replace
gate) are the `go` kind's, unchanged.

## Build configuration, and how it differs from upstream

Upstream `scripts/download` + `scripts/build` + `scripts/package-cli`:

| | upstream release | bashy build |
|---|---|---|
| network fetches at build time | k3s-root tarball (busybox, iptables, conntrack, … — GPL userland), runc clone, containerd clone, Helm charts from k3s.io | **none** beyond the Go modules `go.sum` pins |
| module source | module cache | **`vendor/`** written by `go mod vendor` from the pinned (and overridden) `go.mod`/`go.sum`, `-mod=vendor`; the shim, which k3s does not import, from the module cache (`-mod=readonly`) |
| `k3s` | `./cmd/server`, cgo, static (`-extldflags -static`) in an alpine (musl) image | **same package, `CGO_ENABLED=0`**: upstream tags minus the cgo-only `libsqlite3`/`sqlite_omit_load_extension`/`netcgo`, plus containerd's `no_btrfs` and k3s's `no_cri_dockerd`; static by construction, no C toolchain |
| sqlite / datastore | `mattn/go-sqlite3` (cgo) against system `libsqlite3`; sqlite is the default datastore | **not linked**: kine's `!cgo` driver compiles sqlite out. The datastore is **embedded etcd** — the image must run `k3s server --cluster-init` |
| compiled out | — | btrfs and nix snapshotters, embedded registry mirror (spegel), Docker runtime (cri-dockerd), CRI encrypted-image decryption — see the regression map |
| `containerd-shim-runc-v2` | from a containerd clone, cgo static | from the containerd module k3s's `go.mod` pins (`k3s-io/containerd/v2`), `CGO_ENABLED=0` as containerd ships it |
| `runc` | built with `seccomp` (links libseccomp, LGPL-2.1) | **not built here** — a separate pipeline (`builds/runc`) |
| CNI plugins | `rancher/plugins` + flannel cni-plugin clones | **not built here** — separate pipelines |
| embedded manifests | all of `manifests/` | only `ccm.yaml rolebindings.yaml runtimes.yaml` (`K3S_MANIFESTS`): the ones that reference no image. coredns, local-storage, metrics-server and traefik are not in the binary |
| self-extracting data dir | `cmd/k3s` wrapper embedding the above | **none**: the release is the bare multicall binary; the image lays out `containerd`/`kubectl`/`crictl`/`ctr` as links to it |
| ldflags | `VERSIONFLAGS` incl. build date | the same stamps from the pin; no build date, no cri-dockerd/flannel/kube-router/helm-job stamps |

The build fails if a binary has an ELF interpreter (not static), if
`go version -m` shows `CGO_ENABLED=1` or `mattn/go-sqlite3`, if any
`K3S_FORBIDDEN` path is linked (the tripwire, below), if `k3s --version` does
not report the pinned tag, or if the build changed `go.mod`/`go.sum` after the
gated override step.

### Why CGO_ENABLED=0

bashy uses pure-Go sqlite only and never `mattn/go-sqlite3`. kine (k3s's
datastore shim) already has a `!cgo` path that compiles its sqlite driver out;
it cannot take `modernc.org/sqlite` without patching (it registers the
`sqlite3` driver name `mattn/go-sqlite3` provides), so the datastore is
embedded etcd instead. **runc stays the one cgo build** in the runtime image:
its `nsenter` is C and runs before the Go runtime starts (`builds/runc`).

### The overlay

Tags alone do not get k3s to build without cgo, nor without the no-fix
advisory paths: some upstream files import a removed package
unconditionally. `files/k3s-overlay.sh SRC OUTDIR` writes edited copies of
exactly those files and a `go build -overlay` map (`OUTDIR/overlay.json`),
plus `OUTDIR/apply.tsv` (`TARGET<TAB>BACKING`, empty BACKING = the file is
deleted) for the audit job. It never edits the checkout or `vendor/`.

**Every replaced file is identified by the SHA-256 of its exact upstream
text** (the tag's file, or for the containerd file, the version `go.mod`
pins), and every edit names the line number and the exact text it removes
or replaces. A new tag or module version that changes any of these files
fails the overlay step before anything is built; the edit is then re-reviewed
against the new text and the hash updated. `go` refuses to overlay a file
under `GOMODCACHE`, which is why the build vendors: the containerd file is
overlaid at its `vendor/` path.

The audit job runs the same script and applies `apply.tsv` to its own
checkout (`cp`/`rm`), because govulncheck type-checks from disk and ignores
`-overlay`, so the audited source and tags are the shipped ones.

## Removed and changed vs upstream k3s — regression map

One row per change. *Upstream* is the file or package in k3s `v1.37.1+k3s1`
(or the module `go.mod` pins); *mechanism* is exactly what this build does;
*detect* is the gate that fails if the change regresses; *restore* is what a
consumer who needs the feature must do (always: a different build — this
release cannot be configured back into the feature). Symptoms are read from
the code paths at the pinned tag; the Linux-only binary was not run on the
build host. Overlay line numbers are
at the pinned text, which the overlay verifies by SHA-256. Commits are in this
repository.

| # | change | upstream file / package | our mechanism | advisories | user-visible effect / symptom | detect | restore | commit |
|---|---|---|---|---|---|---|---|---|
| 1 | **cgo off** | `scripts/build` (`CGO_ENABLED=1`, musl static) | `GO_CGO=…=0`, `CGO_ENABLED=0` for both binaries; tags `libsqlite3`, `sqlite_omit_load_extension`, `netcgo` dropped (`GO_BUILD_TAGS`) | — | none by itself; it forces rows 2–4 | build.yml: `go version -m` shows `CGO_ENABLED=1` → fail; `readelf` finds an ELF interpreter → fail | rebuild with cgo (and musl) — then rows 2–4 can return | `0ee47db1` |
| 2 | **sqlite → embedded etcd** | kine `pkg/drivers/sqlite` (`mattn/go-sqlite3`) | not linked: kine's own `!cgo` file compiles the driver out | — | `k3s server` with no `--cluster-init` and no `--datastore-endpoint` does not start (no default datastore) | build.yml: `mattn/go-sqlite3` in `go version -m` → fail | run `--cluster-init` (embedded etcd) or an external `--datastore-endpoint`; sqlite needs a cgo build | `0ee47db1` |
| 3 | **btrfs snapshotter** | `pkg/containerd/builtins_linux.go` imports `containerd/v2/plugins/snapshots/btrfs/plugin` (cgo-only) | tag `no_btrfs` + `files/k3s-overlay.sh` drops line 29 of `pkg/containerd/builtins_linux.go` (sha256 `725c4a27…`) | — | `--snapshotter=btrfs`: k3s does not validate it, and containerd has no `btrfs` snapshotter registered, so the CRI cannot unpack images or create containers | overlay sha256 check; build fails if the import returns (no buildable files without cgo) | a cgo build | `0ee47db1` |
| 4 | **gspt / process title** | `pkg/proctitle/proctile.go` (`var SetProcTitle = gspt.SetProcTitle`, `erikdubbelboer/gspt` is cgo-only) | `files/k3s-overlay.sh` replaces `pkg/proctitle/proctile.go` (sha256 `58662b43…`) with a no-op `func SetProcTitle(cmd string) {}`, like upstream's windows file | — | cosmetic: `ps` shows the original argv (`k3s server …`), not k3s's rewritten title (`k3s server`/`k3s agent`) | overlay sha256 check (v1.37 changed the file from a func to a `var`; the previous exact-text check refuses it) | a cgo build | `0ee47db1`; v1.37 text `bf9f6fc7` |
| 5 | **spegel / embedded registry mirror** | `pkg/spegel/{bootstrap,registry,spegel,store}.go` → `spegel-org/spegel` (`k3s-io/spegel`) → `libp2p/go-libp2p-kad-dht` | `files/k3s-overlay.sh` deletes `pkg/spegel/bootstrap.go`, `registry.go`, `store.go` and replaces `pkg/spegel/spegel.go` (sha256s `f683aa4e…`, `10c891ad…`, `09ac6564…`, `21d0ce4b…`) with one stub keeping the API (`Config`, `DefaultRegistry`, `New*Bootstrapper`, `InjectMirror`, `Start`, `Ready`) | GO-2024-3218 (kad-dht, no fix) | `--embedded-registry` is accepted by the CLI, but the agent fails at startup: `failed to start embedded registry: the embedded registry mirror (--embedded-registry, spegel) is not compiled into this k3s build`. With `--disable-agent` the flag has no effect (the mirror runs in the agent). No `/v1-k3s/p2p` peers, no `p2p.k3s.cattle.io` labels | tripwire: `spegel-org/spegel`, `k3s-io/spegel`, `libp2p/go-libp2p-kad-dht` in `go version -m` or the package graph → fail | use a registry mirror via `registries.yaml`, or an upstream build | `bf9f6fc7` |
| 6 | **Docker runtime / cri-dockerd** | `pkg/agent/cridockerd` → `Mirantis/cri-dockerd`, `docker/docker/client` | k3s's own tag **`no_cri_dockerd`** (`GO_BUILD_TAGS`): `pkg/agent/cridockerd/nocridockerd.go` replaces the package | GO-2026-4883, GO-2026-4887 (`github.com/docker/docker`, no fix) | `k3s agent --docker` (or `server --docker`) fails: `cri-dockerd disabled at build time` (k3s's own message) | tripwire: `Mirantis/cri-dockerd`, `docker/docker` → fail | use containerd (the default) or `--container-runtime-endpoint` to an external CRI | `bf9f6fc7` |
| 7 | **docker/docker's other importers: nix snapshotter + kernel check** | `pkg/containerd/builtins_linux.go` (nix plugin import), `pkg/containerd/utility_linux.go` and `pkg/agent/containerd/config_linux.go` (`nix.Supported`; `docker/docker/pkg/parsers/kernel`). `pdtpartners/nix-snapshotter` imports `docker/docker/errdefs` | `files/k3s-overlay.sh`: drops line 37 of `pkg/containerd/builtins_linux.go`; in `pkg/containerd/utility_linux.go` (sha256 `fd5c3359…`) drops line 12, line 31 returns an error; in `pkg/agent/containerd/config_linux.go` (sha256 `b093a48d…`) drops lines 15 and 21, line 91 calls `bashyCheckKernelVersion`, line 142 returns an error; adds `pkg/agent/containerd/zz_bashy_kernel_linux.go` (the same uname comparison, same `true`-on-error behavior) | GO-2026-4883, GO-2026-4887 | `--snapshotter=nix` fails: `"nix" snapshotter cannot be enabled for …: the nix snapshotter is not compiled into this k3s build` (after k3s's own `nix-store not found in PATH` check). The ≥4.11 kernel check for unprivileged ports/ICMP is unchanged | tripwire: `pdtpartners/nix-snapshotter`, `docker/docker` → fail | use `overlayfs`/`fuse-overlayfs`/`native`/`stargz` (all still compiled), or an upstream build | `bf9f6fc7` |
| 8 | **openpgp feature: CRI encrypted-image decryption** | containerd (`k3s-io/containerd/v2` v2.3.4-k3s1) `internal/cri/server/images/image_pull.go` → `containerd/imgcrypt/v2` → `containers/ocicrypt` → `golang.org/x/crypto/openpgp` (traced with `go list -deps`: `pkg/containerd` → `containerd/v2/plugins/cri` → `internal/cri/server/images` → `imgcrypt/v2/images/encryption` → `ocicrypt` → `x/crypto/openpgp`) | `files/k3s-overlay.sh` edits `vendor/github.com/containerd/containerd/v2/internal/cri/server/images/image_pull.go` (sha256 `a8c276e0…`): drops the two imgcrypt imports (lines 35–36) and the `KeyModelNode` decrypt-on-unpack option (lines 687–692); `encryptedImagesPullOpts` returns nil | GO-2026-5932 (`x/crypto/openpgp`, no fix) | pulling an **encrypted** image (`…+encrypted` layer media types, `image_decryption` key model `node`, the default) fails at unpack: no decrypting unpack option is installed, so containerd cannot apply the encrypted layers. Unencrypted images are unaffected | tripwire: `containerd/imgcrypt`, `containers/ocicrypt`, `golang.org/x/crypto/openpgp` (package graph) → fail | decrypt images before pushing them, or an upstream build | `bf9f6fc7` |
| 9 | **`golang.org/x/net` replace bump** | `go.mod`: `golang.org/x/net => golang.org/x/net v0.57.0` | `GO_SECURITY_OVERRIDES` `replace:golang.org/x/net@v0.60.0` (and its require line), under the replace gate | GO-2026-6603, -6610, -6611, -6612, -6617 | none intended (HTTP/2 fixes) | govulncheck (audit job) reports them reachable if the bump is lost; the replace gate refuses any other replace change | drop the entry once an upstream tag pins ≥ v0.60.0 (the step then logs "already satisfied — retire the override") | `bf9f6fc7` (rule `afb85964`) |
| 10 | **`github.com/cilium/ebpf` replace bump** | `go.mod`: `github.com/cilium/ebpf => github.com/cilium/ebpf v0.12.3` (used via `containerd/cgroups/v3/cgroup2`, `pkg/cgroups`) | `GO_SECURITY_OVERRIDES` `replace:github.com/cilium/ebpf@v0.22.0` (same major v0; it compiles for linux amd64 and arm64) | GO-2026-6238 | none intended; cgroup v2 device-filter eBPF programs now come from ebpf v0.22.0 — a cgroup v2 device-access regression would surface here | as row 9 | as row 9 | `bf9f6fc7` (rule `afb85964`) |
| 11 | **require lines moved with rows 9–10** | `go.mod` require lines of `docker/docker-credential-helpers` v0.9.3, `google/go-containerregistry` v0.20.6, `x/crypto` v0.57.0, `x/mod` v0.41.0, `x/sync` v0.23.0, `x/sys` v0.48.0, `x/telemetry`, `x/term` v0.46.0, `x/text` v0.42.0, `x/tools` v0.49.0 | declared `GO_SECURITY_OVERRIDES` entries at the version MVS selects for rows 9–10 (`x/sys` and `x/crypto` stay replaced by k3s to v0.47.0/v0.54.0, so those two are inert) | — | none intended | the override gate refuses any undeclared go.mod change; `-mod=vendor` fails on an inconsistent go.mod | regenerate the list when the pin moves | `bf9f6fc7` |
| 12 | **vendor build** | — | `go mod vendor` in the build and audit jobs, k3s built `-mod=vendor` | — | none (same module versions) | go.mod/go.sum hash recorded after the override step must match after the build | — | `bf9f6fc7` |
| 13 | **image-free manifests only** | `manifests/` (all, copied by `scripts/build`) | only `K3S_MANIFESTS="ccm.yaml rolebindings.yaml runtimes.yaml"` copied into `pkg/deploy/embed/`; build fails if one names an `image:` | — | coredns, local-storage, metrics-server and traefik are never deployed by k3s (cluster DNS, the `local-path` StorageClass, `kubectl top` and ingress are absent until the consumer deploys its own) | build.yml manifest step (`image:` → fail) | deploy those from bashy-built images | `a28ca109` |
| 14 | **add-ons and helm-controller** | `scripts/download` (Helm charts from k3s.io), `scripts/build` ldflag `helm-controller …/chart.DefaultJobImage=rancher/klipper-helm:<version>` | no charts fetched or embedded; the `DefaultJobImage` stamp is not set; no k3s-root userland, runc, CNI plugins or self-extracting data dir | — | helm-controller still runs but has no bundled HelmChart to install; a user-created `HelmChart` would launch `rancher/klipper-helm:latest` (unpinned upstream image) — run the server with `--disable-helm-controller` (and `--disable=traefik,servicelb,metrics-server`) unless you supply a bashy-built job image | — (declaration) | supply the job image via the HelmChart `jobImage` field, or an upstream build | `a28ca109` |

The **tripwire** is `K3S_FORBIDDEN` in `tool.env`:
`github.com/spegel-org/spegel github.com/k3s-io/spegel github.com/libp2p/go-libp2p-kad-dht github.com/docker/docker golang.org/x/crypto/openpgp github.com/containers/ocicrypt github.com/containerd/imgcrypt github.com/Mirantis/cri-dockerd github.com/pdtpartners/nix-snapshotter`.
After building, build.yml lists every module `go version -m` reports for
**both** binaries (the `dep` and `=>` lines) and every package of the graph
they were built from (`go list -deps` with the same tags, `-mod=vendor` and
overlay for k3s, `-mod=readonly` for the shim), and fails the leg if any path
equals, or is under, a `K3S_FORBIDDEN` entry. The package graph is what makes
`golang.org/x/crypto/openpgp` checkable at all: it is a package of
`golang.org/x/crypto`, which stays linked. Run against upstream's unmodified
graph, the same check reports all nine prefixes.

### What else in bashy is affected: nothing

The removals live in this k3s binary only. Checked at outpost `05791df`,
bashy `cmd/bashy` and the pinned yoke:

- **Outpost's runtime entrypoints** (`internal/agent/runtime/image/`) never
  pass `--embedded-registry`, `--docker`, `--snapshotter=nix`, write no k3s
  `config.yaml` or `registries.yaml`, and configure no `image_decryption`.
  Every k3s flag they pass: `k3s agent` (`entrypoint.sh`) —
  `--server`, `--token`, `--node-name`, `--node-label` (×3), `--with-node-id`,
  `--disable-apiserver-lb`, `--resolv-conf`, `--snapshotter=${SNAPSHOTTER:-fuse-overlayfs}`
  (`overlayfs` as the operator override), `--kubelet-arg=port=…` (if set),
  `--flannel-iface=tailscale0` (peer-flannel mode),
  `--kubelet-arg=address=127.0.0.1`,
  `--kubelet-arg=feature-gates=KubeletInUserNamespace=true`,
  `--kubelet-arg=cgroups-per-qos=false`, `--kubelet-arg=enforce-node-allocatable=`;
  `k3s server` (`server-entrypoint.sh`) — `--disable-agent`,
  `--disable-cloud-controller`, `--disable=traefik,servicelb,metrics-server`,
  `--egress-selector-mode=disabled`, `--write-kubeconfig`,
  `--write-kubeconfig-mode=644`, `--https-listen-port`, `--tls-san` (×2 plus
  each `OUTPOST_TLS_SAN` entry), `--advertise-address`,
  `--kube-apiserver-arg=kubelet-preferred-address-types=ExternalIP`,
  `--kube-apiserver-arg=kubelet-certificate-authority=`, and `--cluster-cidr`/
  `--service-cidr` when set. `fuse-overlayfs` and `overlayfs` are still
  compiled. Note the server runs without `--cluster-init` and the image
  currently uses the upstream release (sqlite); row 2 applies when it moves
  to this build.
- **Our binaries do not import the removed packages.** `go list -deps`
  (linux/amd64 `CGO_ENABLED=0` and the host): `outpost ./cmd/outpost`,
  `bashy ./cmd/bashy` (default and `embed_runner`/`bashy_scratch` tags) and
  yoke (`cmd/yoke`, `cmd/yoke-seedfit`) contain none of `go-libp2p-kad-dht`,
  spegel, any `docker/docker` package, `x/crypto/openpgp`, `ocicrypt`,
  `imgcrypt`, `cri-dockerd`, `nix-snapshotter`, nor any k3s,
  `k8s.io/kubernetes` or containerd package — so k3s build tags cannot reach
  them. Outpost's sphere/mesh (`internal/agent/mesh`) uses go-libp2p **core**
  (`core/{crypto,host,network,peer,peerstore}`, `p2p/discovery/mdns`,
  `p2p/host/peerstore/pstoremem`, `p2p/net/swarm`,
  `p2p/protocol/circuitv2/client`) and its own rendezvous — no kad-dht — in
  its own binary, which is separate from k3s. The one place `ocicrypt` and
  `x/crypto/openpgp` appear is bashy's opt-in `bashy_engines` build (podman/
  buildah image signing via yoke's `external/podman/engine`), unrelated to k3s
  and to this change.

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
- Do not pass `--embedded-registry`, `--docker` or `--snapshotter=nix`, and
  do not pull encrypted images (regression map rows 5–8).

## Audit

`audit.yml` generates the same `vendor/`, applies the overlay to its checkout,
and runs govulncheck in source mode over both shipped packages with the k3s
build tags and `CGO_ENABLED=0`: `./cmd/server` with `-mod=vendor`, the shim
(not vendored) with `-mod=readonly`, both JSON streams in one report (the job
fails unless both analyzed). The SBOM is `cyclonedx-gomod app` of
`./cmd/server` with `-mod=vendor`, so it lists exactly the modules the binary
links (none of `K3S_FORBIDDEN`). Reachable, unwaived findings fail the
release; `files/vuln-ignore` is empty.

### Result at this pin

govulncheck v1.9.0, go1.26.9, with the overrides, tags and overlay:
**0 reachable** (376 modules for k3s, 33 for the shim; `golang.org/x/net`
v0.60.0, `github.com/cilium/ebpf` v0.22.0). Upstream's graph at the same tag
had 12 reachable advisories: the five `x/net` ones and GO-2026-6238 (fixed by
rows 9–10), GO-2024-3218, GO-2026-4883, -4887 and -5932 (no fix; compiled out
by rows 5–8), and `pion/dtls/v3` GO-2026-6165 / `quic-go/webtransport-go`
GO-2026-6099, which were reachable only through spegel's libp2p and left the
graph with row 5. Informational, not reachable: GO-2026-5932, -6303, -6354,
-6355 (`golang.org/x/crypto` module, packages not imported) and GO-2026-6094
(`google/cel-go`).

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
git commit -m "bashy-build: k3s v1.37.1+k3s1 verified build"
git push -u origin bashy-build
git tag bashy-v1.37.1+k3s1-1 && git push origin bashy-v1.37.1+k3s1-1
```

Then pin the release in binmgr against the signing identity and issuer above.
