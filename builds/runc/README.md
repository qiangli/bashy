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
| License at the tag | **Apache-2.0** (`LICENSE`, verified at `v1.5.2`) |
| Pinned release | `v1.5.2` (annotated tag `b622a599…`) → commit `29dd3dc2b13b4123162e5fe132504bb4b15569f1` |
| Toolchain | Go `1.25.0` (the tag's `go.mod`), built with the EOL-minor security override `1.26.9` |
| Trigger | push of tag `bashy-v1.5.2-N` on `bashy-build` (first: `bashy-v1.5.2-1`) |
| Signing identity | `https://github.com/qiangli/runc/.github/workflows/bashy-build.yml@refs/tags/bashy-v1.5.2-1` |
| Issuer | `https://token.actions.githubusercontent.com` |

## Layout

```
tool.env                  the declaration (shipped as PIN)
build.yml                 static musl cgo build from vendor/ (replaces the go kind's build.yml)
files/no-egress-test.sh   runtime gate: CLI verbs plus a real container run
files/otel.env            OpenTelemetry environment (disabled; local collector only)
files/no-tracking.env     proxy clearing
files/vuln-ignore         govulncheck waivers (empty)
```

## Build configuration, and how it differs from upstream

Upstream releases come from `make static`: cgo on, `-tags "seccomp
urfave_cli_no_docs libpathrs netgo osusergo"`, static-pie against a glibc, a
libseccomp and a libpathrs built from source. The bashy build is the same package with these
differences:

| | upstream release | bashy build |
|---|---|---|
| tags | `seccomp urfave_cli_no_docs libpathrs netgo osusergo` | `urfave_cli_no_docs netgo osusergo` — **no `seccomp`, no `libpathrs`** |
| libc | glibc (LGPL), static-pie | musl (MIT) via `musl-gcc`, `-linkmode external -extldflags -static` |
| libseccomp | linked (LGPL-2.1) | not linked: without the tag runc compiles `seccomp_unsupported.go` and `libseccomp-golang` drops out of the graph |
| libpathrs | linked (Rust, C ABI) via `cyphar.com/go-pathrs` | not linked: without the tag runc uses the pure-Go `pathrs-lite` from `filepath-securejoin` |
| toolchain | the release host's | go1.26.9 (go1.25 is end-of-life) |
| modules | the tag's `vendor/` | the tag's `vendor/` (`-mod=vendor`), unmodified; nothing fetched |
| ldflags | `main.gitCommit` from `git describe` | `main.gitCommit` = the pinned commit; `-s -w -buildid=` |

cgo is needed for exactly what upstream needs it for: `libcontainer/nsenter`
(the C that runs before the Go runtime in `runc init`) and
`libcontainer/internal/userns`. The build fails if the binary has an ELF
interpreter, if `go version -m` does not record `CGO_ENABLED=1`, if anything
seccomp-shaped or libpathrs is tagged or linked, if `--version` does not
report the pinned version or reports a libseccomp, or if the build changed
`go.mod`/`go.sum`/`vendor/`.

**Consequence for the image:** this runc does not enforce seccomp profiles.
A container spec that requests one is run without it.

## Audit

The `go` kind's audit job, unchanged: `GO_BUILD_ENV="CGO_ENABLED=1"` puts the
cgo files (nsenter, userns) in the graph, the build tags are the shipped ones,
and modules come from the tag's `vendor/`. govulncheck v1.9.0 of `.` under
go1.26.9 is **0 reachable, nothing waived**, with no `GO_SECURITY_OVERRIDES`:
v1.5.2 already requires `opencontainers/cgroups` v0.1.0, which no longer
reaches the `cilium/ebpf` BTF finding (GO-2026-6238) that blocked v1.4.2
without an override.

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
git commit -m "bashy-build: runc v1.5.2 verified build"
git push -u origin bashy-build
git tag bashy-v1.5.2-1 && git push origin bashy-v1.5.2-1
```

Then pin the release in binmgr against the signing identity and issuer above.
