---
name: bashy
description: Build/test/lint targets for bashy, as a bashy dag pipeline (dogfood of the Makefile)
---

# bashy — DAG task file

The agent-first equivalent of this repo's `Makefile`, runnable with the
`bashy dag` task runner this repo's AgentOS shell ships:

```bash
./bashy dag --list           # fresh checkout bootstrap: builds bin/bashy if needed
./bashy dag build            # build both binaries
./bashy dag install          # install bash/bashy into GOBIN, after which `bashy dag ...` works
make dag ARGS=build          # make-based bootstrap wrapper around ./bashy dag
bashy dag test               # once installed/on PATH: go test ./...
bashy dag --json test        # machine-readable envelope for an agent
```

Chicken/egg note: `bashy dag ...` requires the operating system to find a
`bashy` executable first. From a fresh source checkout, use the repo-local
`./bashy` launcher; it builds `bin/bashy` if missing and then re-execs it.
Inside target bodies, call `"$BASHY" ...` when you need bashy itself. Mirroring
GNU Bash's `BASH`/`BASH_ARGV0` split, the DAG runner sets `BASHY`/`BASHY_EXE`
to the resolved executable path for the `argv[0]` that launched the run, and
`BASHY_ARGV0` to the raw argv0 string. Recursive calls should use `"$BASHY"`
so they stay on the same binary version instead of a stale `bashy` elsewhere on
`PATH`.

Targets carry `Requires:` (dependency edges), `Sources:`/`Generates:`
(content-fingerprint up-to-date skip — `bashy dag build` no-ops when nothing
changed; `--force`/`-B` re-runs) and `Effects:` (capability cap, recorded in
the attestation; `Ensure:` postconditions are enforced too). Targets run in
topological order through the in-process shell — add `-j N` for parallel.

## Tasks

### build
Build both independent binaries into bin/ (bash = pure drop-in from cmd/bash;
bashy = AgentOS shell from cmd/bashy). Separate compilations — bash's import
graph never includes coreutils. This is the **lean worker** bashy: shell +
coreutils userland + git + dag + `bashy go` (self-provisioning Go toolchain) +
weave; ~121 MB unix, ~47 MB Windows (it cross-compiles everywhere — podman/ollama
are !windows-gated, the otel observability stack is off by default). For a host
build with the observability stack, use `build-host`.
Sources: cmd/, internal/, go.mod, go.sum
Generates: bin/bash, bin/bashy
Effects: write

```bash
set -e
mkdir -p bin
VERSION="${VERSION:-dev}"
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}'"
go build -trimpath -ldflags "$LDFLAGS" -o bin/bash  ./cmd/bash
go build -trimpath -ldflags "$LDFLAGS" -o bin/bashy ./cmd/bashy
```

### build-host
Build the full **unix host** bashy: the container/LLM engines (`bashy
podman`/`ollama`, `-tags bashy_engines`, cgo + btrfs/MLX) and the observability
stack (`bashy otel`, `-tags bashy_obs`, ~193 MB). Not cross-platform — use only
on a host node; the default `build` is the lean cross-platform worker.
Generates: bin/bashy
Effects: write

```bash
set -e
mkdir -p bin
VERSION="${VERSION:-dev}"
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}'"
go build -trimpath -tags "bashy_engines bashy_obs" -ldflags "$LDFLAGS" -o bin/bashy ./cmd/bashy
```

### install
go install both binaries into GOBIN.
Effects: write

```bash
VERSION="${VERSION:-dev}"
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}'"
go install -trimpath -ldflags "$LDFLAGS" ./cmd/bash ./cmd/bashy
```

### test
Run all Go tests.
Effects: read

```bash
go test ./...
```

### dist
Cross-compile static binaries for all release platforms into bin/dist/ (both
bash and bashy; a local cross-compile sanity check — goreleaser does real
releases).
Generates: bin/dist
Effects: write

```bash
set -e
mkdir -p bin/dist
VERSION="${VERSION:-dev}"
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}'"
for plat in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os=${plat%/*}; arch=${plat#*/}; ext=""
  [ "$os" = windows ] && ext=.exe
  for name in bash bashy; do
    out="bin/dist/${name}-${os}-${arch}${ext}"
    echo "building $out..."
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -trimpath -ldflags "$LDFLAGS" -o "$out" "./cmd/${name}"
  done
done
```

### test-bash
**GNU Bash 5.3 compatibility test** — the canonical conformance gate. Runs
Bash's own 5.3 test suite against the freshly built `bin/bash` (the pure
drop-in); the headline is the PASS-count three-tuple (currently **86/86**, 0
fail, 0 skip). The 130-line compliance harness (per-fixture timeouts,
expect-line filtering, cat -v transforms, C helper compilation) stays in the
Makefile — this target runs `build` first, then delegates to it. Use
`make test-bash-parallel` for the cores-fanned-out form (the canonical gate).
Re-home into pure dag bodies once the harness is factored into a script.
Requires: build
Effects: write

```bash
make test-bash
```

### yash
**yash POSIX (`-p`) conformance test** — the POSIX-conformance frontier metric
(an INFO probe, not a 0/1 gate). Cross-builds the pure `cmd/bash` for the
container arch, clones yash's GPL test suite into a gitignored cache
(`.yash-tests/`, never vendored), and runs the testee in POSIX mode across two
oracle panels (alpine: bash 5.3/dash/ash/yash/mksh/loksh/zsh; debian adds
posh/ksh93), reporting bashy's pass rate vs each. Needs a container engine
(`bashy podman` on a unix host, or docker). As of 2026-06-29: **bashy 96%**
(alpine 1763/1826, debian 1777/1838) — ahead of bash (95% / 94%) and tied with
mksh for best. See `docs/cross-shell-conformance-baseline.md`.
Effects: write

```bash
scripts/yash-posix-suite.sh
```

### tidy
go mod tidy + gofmt -s -w . + go vet ./...
Effects: write

```bash
set -e
go mod tidy
gofmt -s -w .
go vet ./...
```

### clean
Remove built binaries.
Effects: destroy

```bash
rm -rf bin
```
