---
name: bashy
description: Build/test/lint targets for bashy, as a bashy dag pipeline (dogfood of the Makefile)
---

# bashy — DAG task file

The agent-first equivalent of this repo's `Makefile`, runnable with the
`bashy dag` task runner this repo's AgentOS shell ships:

```bash
bashy dag --list            # what `make help` showed
bashy dag build             # build both binaries
bashy dag test              # go test ./...
bashy dag --json test       # machine-readable envelope for an agent
```

Targets carry `Requires:` (dependency edges), `Sources:`/`Generates:`
(content-fingerprint up-to-date skip — `bashy dag build` no-ops when nothing
changed; `--force`/`-B` re-runs) and `Effects:` (capability cap, recorded in
the attestation; `Ensure:` postconditions are enforced too). Targets run in
topological order through the in-process shell — add `-j N` for parallel.

## Tasks

### build
Build both independent binaries into bin/ (bash = pure drop-in from cmd/bash;
bashy = AgentOS shell from cmd/bashy). Separate compilations — bash's import
graph never includes coreutils.
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
Run the Bash 5.3 native test suite against the freshly built `bin/bash`. The
130-line compliance harness (per-fixture timeouts, expect-line filtering,
cat -v transforms, C helper compilation) stays in the Makefile — this target
runs `build` first, then delegates to it. Re-home into pure dag bodies once the
harness is factored into a script.
Requires: build
Effects: write

```bash
make test-bash
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
