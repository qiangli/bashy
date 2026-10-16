# runc — bashy verified build

The outpost runtime image's containerd shim execs runc only from a
**bashy-CI-built, keylessly Sigstore-signed build of the official upstream
release tag**. This directory is the declaration for that build. `fork/` is
installed as the `bashy-build` branch of `github.com/qiangli/runc`; it carries
no upstream source — the workflow checks out the upstream tag fresh on every
run.

| | |
|---|---|
| Upstream | `https://github.com/opencontainers/runc` |
| License at the tag | **Apache-2.0** (`LICENSE`, verified at `v1.4.2`) |
| Pinned release | `v1.4.2` (annotated tag `c909e4c2…`) → commit `c241c0bb5e60a8e8c1b2e53d4eca8d0068d8d57e` |
| Toolchain | Go `1.24.0` (the tag's `go.mod`), built with the EOL-minor security override `1.26.9` |
| Trigger | push of tag `bashy-v1.4.2-N` on `bashy-build` (first: `bashy-v1.4.2-1`) |
| Signing identity | `https://github.com/qiangli/runc/.github/workflows/bashy-build.yml@refs/tags/bashy-v1.4.2-1` |
| Issuer | `https://token.actions.githubusercontent.com` |

## Layout

```
tool.env                  the declaration (shipped as PIN)
build.yml                 static musl cgo build (replaces the go kind's build.yml)
audit.yml                 govulncheck + SBOM with cgo on and -mod=mod
files/no-egress-test.sh   runtime gate: CLI verbs plus a real container run
files/otel.env            OpenTelemetry environment (disabled; local collector only)
files/no-tracking.env     proxy clearing
files/vuln-ignore         govulncheck waivers (empty)
```

## Build configuration, and how it differs from upstream

Upstream releases come from `make static`: cgo on, `-tags "seccomp
urfave_cli_no_docs netgo osusergo"`, static-pie against a glibc and a
libseccomp built from source. The bashy build is the same package with these
differences:

| | upstream release | bashy build |
|---|---|---|
| tags | `seccomp urfave_cli_no_docs netgo osusergo` | `urfave_cli_no_docs netgo osusergo` — **no `seccomp`** |
| libc | glibc (LGPL), static-pie | musl (MIT) via `musl-gcc`, `-linkmode external -extldflags -static` |
| libseccomp | linked (LGPL-2.1) | not linked: without the tag runc compiles `seccomp_unsupported.go` and `libseccomp-golang` drops out of the graph |
| toolchain | the release host's | go1.26.9 (go1.24 is end-of-life) |
| modules | the tag's `vendor/` | the tag's `go.mod` plus one reviewed override (§Audit), resolved from `go.sum` with `-mod=mod` |
| ldflags | `main.gitCommit` from `git describe` | `main.gitCommit` = the pinned commit; `-s -w -buildid=` |

cgo is needed for exactly what upstream needs it for: `libcontainer/nsenter`
(the C that runs before the Go runtime in `runc init`) and
`libcontainer/internal/userns`. The build fails if the binary has an ELF
interpreter, if `go version -m` does not record `CGO_ENABLED=1`, if anything
seccomp-shaped is tagged or linked, if `--version` does not report the pinned
version or reports a libseccomp, or if the build rewrote `go.mod`/`go.sum`.

**Consequence for the image:** this runc does not enforce seccomp profiles.
A container spec that requests one is run without it.

## Audit

govulncheck v1.9.0 of `.` under go1.26.9, cgo on, with the build tags. At the
tag's own `go.mod` one module finding is reachable: GO-2026-6238 (BTF parsing
in `github.com/cilium/ebpf` v0.17.3), via `opencontainers/cgroups` v0.0.6's
`devices.haveBpfProgReplace` → `ebpf.NewProgram`. The fixed ebpf (v0.22.0)
removed API that every released `opencontainers/cgroups` uses, so it cannot be
taken. `GO_SECURITY_OVERRIDES="github.com/opencontainers/cgroups@v0.1.0"` (the
cgroups runc v1.5.x requires) moves one require line, within its major, adds
none and leaves the `go` directive alone; the tree builds and govulncheck is
then **0 reachable, nothing waived**.

Because the override moves `go.mod` past `vendor/modules.txt`, build and audit
use `-mod=mod`: modules come from the proxy, verified against `go.sum`, not
from the tag's vendor tree. The override retires when the pin moves to a runc
whose `go.mod` already carries cgroups v0.1.0 (v1.5.x).

## Release assets

`runc-linux-{amd64,arm64}`, `runc-<tag>.cdx.json`,
`runc-<tag>.govulncheck.json`, `runc-pin.txt`, `runc-otel.env`,
`runc-no-tracking.env`, `vuln-ignore`, each with a `.sigstore.json` bundle,
plus `SHA256SUMS`.

## Operator steps (not in bashy CI)

```sh
# in a clone of github.com/qiangli/runc
git switch --orphan bashy-build
cp -R <bashy>/builds/runc/fork/. .
git add .github/workflows/bashy-build.yml bashy-build
git commit -m "bashy-build: runc v1.4.2 verified build"
git push -u origin bashy-build
git tag bashy-v1.4.2-1 && git push origin bashy-v1.4.2-1
```

Then pin the release in binmgr against the signing identity and issuer above.
