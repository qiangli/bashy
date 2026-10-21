# Runtime-acquisition inventory (download + exec; NOT linked)

Status: Sprint 350 story 476b77ff9e24, reconciled against live callsites
2026-10-21. Policy: `licensing-supply-chain-policy.md` §2 — download + exec is
not bundling. Nothing here appears in the SBOMs (`docs/license-inventory.md`,
`dist/sbom/manifest.json`), which list only what is **linked into a release
binary**. A row here is never evidence of a linked dependency.

"Redistributes bytes" = Bashy's release artifacts or images carry the bytes.
Every row below is **no** unless stated: the program is fetched from the
upstream origin into the binmgr cache (`$BASHY_BIN_CACHE`, default
`<UserCacheDir>/bashy/bin`) on first use and executed as a child process.

| Program | Bashy trigger (callsite) | Platform | Version pin | Source | Digest mechanism | Cache destination | Actual-asset license (as read) | Redistributes |
|---|---|---|---|---|---|---|---|---|
| podman-static tree (podman, conmon, crun/runc, netavark, aardvark-dns, pasta) | `bashy podman` → `podmanTool()` `internal/agentos/engines_podman.go` | linux/amd64, linux/arm64 | pinned `v6.1.2` | github.com/mgoltzsche/podman-static releases | sha256 committed in `podmanAssets` (reviewed-history trust root), verified by `binmgr.Ensure` | `<cache>/podman/<ver>/` (tree) | Apache-2.0 project; **crun, pasta, fuse-overlayfs GPL-2.0** — separate processes | no |
| podman remote client | `bashy podman` (same) | darwin/arm64, windows/amd64, windows/arm64 `v6.1.2`; darwin/amd64 `v5.8.7` | pinned | github.com/containers/podman releases | sha256 in `podmanAssets` | `<cache>/podman/<ver>/` | Apache-2.0 | no |
| gvproxy, vfkit | `provisionDarwinHelpers` (`engines_podman.go`) | darwin | `v0.8.8`, `v0.6.4` | containers/gvisor-tap-vsock, crc-org/vfkit releases | sha256 in `darwinHelperAssets` | `<cache>/podman-helpers/` | Apache-2.0 | no |
| otel stack (OTel Collector, VictoriaMetrics/Logs, Jaeger, Perses) | `bashy otel` → `resolveOtelBinary` `internal/agentos/obs_stub.go` (lean build; `-tags bashy_obs` links it instead — not a release artifact) | per `otel-<goos>-<goarch>.gz` release blob | dynamic: latest release of `$BASHY_ENGINE_REPO` (default `qiangli/bashy`) | GitHub release of this repo, built in CI from permissive source | `binmgr.ProvisionManaged` (transport check; `$BASHY_OTEL_BIN` overrides) | `engineCacheDir()/otel` | Apache-2.0 components (Collector, VictoriaMetrics, Jaeger, Perses); stack is built by Bashy CI so **Bashy publishes these bytes as a release asset**, never inside `bashy` | blob is a separate release asset, not inside bashy |
| Node.js (+ bundled npm) | `node`/`npm`/`npx`/`pnpm` front doors (`agentos.go`), `typescript` fence (`toolchains.go`) | all | `node.DefaultVersion` 22.23.2 | nodejs.org/dist | pinned sha256 table in yoke `external/node`, drift-checked against live `SHASUMS256.txt`; unpinned versions fetch SHASUMS256 (fail-closed) | `<cache>/node/<ver>/` | MIT core + permissive bundled deps; npm **Artistic-2.0** (permissive, non-copyleft, spelled out) | no |
| TypeScript compiler | `typescript` fence → `node.EnsureTypeScript` | all | `node.DefaultTypeScript` 5.9.3 | npm registry | npm integrity via the provisioned node | node tree | Apache-2.0 | no |
| Bun (+ statically linked JavaScriptCore/WebKit, tinycc) | `BASHPP_TYPESCRIPT_RUNTIME=bun` or bun lockfile → `bun.Ensure` | linux, darwin, windows | `bun-v1.4.2` | github.com/oven-sh/bun releases | release `SHASUMS256.txt` transit check | `<cache>/bun/<ver>/` | own code MIT; **binary statically links LGPL-2 JavaScriptCore/WebKit and LGPL-2.1 tinycc** → not permissive as a program; opt-in only, never default | no |
| Go toolchain | `bashy go`, `go` fence | all | `gotoolchain.DefaultVersion` | go.dev | pinned sha256 | `<cache>/go/` | BSD-3-Clause | no |
| CPython via uv | `python` fence, `bashy uv` | all | uv 0.5.11; Python 3.13 line | astral-sh/uv, python-build-standalone | release digest | `<cache>/uv`, `<cache>/python` | uv Apache-2.0/MIT; CPython PSF-2.0 | no |
| Zig (`zig cc`) | `c`/`cpp` fences, `cc` rung (`toolchains.go`) | all | 0.16.0 | ziglang.org | pinned sha256 | `<cache>/zig/` | MIT; bundles LGPL-2.1 glibc / ZPL-2.1 mingw trees as compile-time inputs | no |
| rustup/rustc | `rust` fence → `rust.EnsureRustc` | all | `stable` | static.rust-lang.org | rustup-init verification | rustup home | Apache-2.0 OR MIT | no |
| goreleaser | `bashy release` → `binmgr.EnsureGoreleaser` (`release.go`) | all | pinned in yoke `binmgr/goreleaser.go` | github.com/goreleaser/goreleaser | pinned digest | `<cache>/goreleaser/` | MIT | no |
| bashy release members (bashy, bash, sh, outpost) | `bashy self update` → `resolveBashyRelease` / `binmgr.EnsureMembers` (`self.go`) | all | latest or requested tag | this repo's GitHub releases | release checksums + attestation | install dir | MIT (these are Bashy's own artifacts) | yes (they are the release) |
| POSIX utility providers (pinned external providers registered by the multicall; system utilities) | coreutils multicall provider table (see `docs/posix-command-coverage.md`: 10 pinned external providers) | all | per provider table in coreutils | per provider | pinned digest in provider table | binmgr cache | per provider — consult `../coreutils/docs/posix-required-commands.md`; GPL providers (if any) are exec'd only | no |
| ollama, gh, act, loom, rclone, zot, seaweedfs, kopia, tofu, kubectl, helm, cmake, clang, git-scm, curl, pwsh | their `bashy <verb>` front doors (yoke `external/*`) | per verb | pinned per verb in yoke `external/<verb>` | upstream releases | pinned sha256 / release shasums | binmgr cache | upstream licenses; none linked | no |
| SearXNG image (AGPL-3.0) | `bashy search` last rung via `bashy podman` | container | upstream image | upstream registry | image digest | podman storage | AGPL-3.0, unmodified, localhost-only (policy §2) | no |

## Reconciliation notes (stale-doc fixes)

- `internal/agentos/toolchains.go` header says Bun is "never the default": confirmed, matches `islandToolchains`.
- The `bashy` embedded blobs (`embed_podman`, `embed_vfkit`, `embed_gvproxy`) are an opt-in host build (`make build-host`); a release built with them **does** redistribute those bytes and must add their rows to `docs/license-inventory.md` evidence. The shipped release matrix does not set them.
- Rows deferred to sibling-owned pin tables (POSIX providers, yoke `external/*`) cite the owning table rather than copying pins, so they cannot drift here; re-verify with the yoke pin at `go.mod`.
