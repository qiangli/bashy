---
name: bashy
description: Build/test/lint targets for bashy, as a bashy dag pipeline (dogfood of the Makefile)
---

# bashy — DAG task file

The agent-first equivalent of this repo's `Makefile`, runnable with the
`bashy dag` task runner this repo's AgentOS shell ships:

```bash
./bashy dag --list           # fresh checkout bootstrap: builds bin/bashy if needed
./bashy dag build            # build both binaries through bashy go
./bashy dag install          # install bash/bashy into GOBIN, after which `bashy dag ...` works
make dag ARGS=build          # make-based bootstrap wrapper around ./bashy dag
bashy dag test               # once installed/on PATH: bashy go test ./...
bashy dag --json test        # machine-readable envelope for an agent
```

Chicken/egg note: `bashy dag ...` requires the operating system to find a
`bashy` executable first. From a fresh source checkout, use the repo-local
`./bashy` launcher; it builds `bin/bashy` if missing and then re-execs it. If an
older bashy is already installed, the launcher uses that binary's `bashy go`
front end to build the checkout-local binary. Otherwise it falls back to a host
Go toolchain, which is the only external dependency needed for a first build
from already-present source.
Inside target bodies, call `"$BASHY" ...` when you need bashy itself. Mirroring
GNU Bash's `BASH`/`BASH_ARGV0` split, the DAG runner sets `BASHY`/`BASHY_EXE`
to the resolved executable path for the `argv[0]` that launched the run, and
`BASHY_ARGV0` to the raw argv0 string. Recursive calls should use `"$BASHY"`
so they stay on the same binary version instead of a stale `bashy` elsewhere on
`PATH`.

Outpost note: once a released bashy is installed, a patch build can stay inside
bashy's own surface: `bashy git` for source checkout, `./bashy dag build` for
the native build through `bashy go`, and `bashy podman run -v "$PWD:/work" -w
/work ...` for containerized build/test lanes when the host should only provide
the already-installed bashy.

Targets carry `Requires:` (dependency edges), `Sources:`/`Generates:`
(content-fingerprint up-to-date skip — `bashy dag build` no-ops when nothing
changed; `--force`/`-B` re-runs) and `Effects:` (capability cap, recorded in
the attestation; `Ensure:` postconditions are enforced too). Targets run in
topological order through the in-process shell — add `-j N` for parallel.

## Tasks

### build
Build both independent binaries into bin/ (bash = pure drop-in from cmd/bash;
bashy = AgentOS shell from cmd/bashy). Separate compilations — bash's import
graph never includes coreutils. The recipe invokes `"$BASHY" go build`, so an
installed or checkout-local bashy owns the Go toolchain path. This is the
**lean worker** bashy: shell + coreutils userland + git + dag + `bashy go`
(self-provisioning Go toolchain) +
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
BASHY_EXE="${BASHY:-bashy}"
goos="${GOOS:-$("$BASHY_EXE" go env GOOS)}"
ext=""
[ "$goos" = windows ] && ext=.exe
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}'"
"$BASHY_EXE" go build -trimpath -ldflags "$LDFLAGS" -o "bin/bash${ext}"  ./cmd/bash
"$BASHY_EXE" go build -trimpath -ldflags "$LDFLAGS" -o "bin/bashy${ext}" ./cmd/bashy
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
BASHY_EXE="${BASHY:-bashy}"
goos="${GOOS:-$("$BASHY_EXE" go env GOOS)}"
ext=""
[ "$goos" = windows ] && ext=.exe
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}'"
"$BASHY_EXE" go build -trimpath -tags "bashy_engines bashy_obs" -ldflags "$LDFLAGS" -o "bin/bashy${ext}" ./cmd/bashy
```

### install
go install both binaries into GOBIN.
Effects: write

```bash
VERSION="${VERSION:-dev}"
BASHY_EXE="${BASHY:-bashy}"
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}'"
"$BASHY_EXE" go install -trimpath -ldflags "$LDFLAGS" ./cmd/bash ./cmd/bashy
```

### test
Run all Go tests.
Effects: read

```bash
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go test ./...
```

### test-podman
Build a platform-appropriate bashy Podman engine binary and smoke-test the
container path. This target is intentionally runnable on every platform:
Linux/macOS run the engine build directly; Windows builds the WSL-backed remote
engine (`bashy_engines remote containers_image_openpgp`). If the host substrate
is not ready yet (for example Windows has just enabled WSL/VMP and needs a
reboot, or Hyper-V is selected but not enabled), the target reports a SKIP with
the reason instead of failing unrelated DAG runs. Set
`CONTAINERS_MACHINE_PROVIDER=hyperv` to exercise Hyper-V on Windows instead of
the default WSL provider.
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
goos="$("$BASHY_EXE" go env GOOS)"
ext=""
[ "$goos" = windows ] && ext=.exe
tags="bashy_engines"
[ "$goos" = windows ] && tags="bashy_engines remote containers_image_openpgp"

if [ -d ../coreutils/.git ]; then
  git -C ../coreutils submodule update --init external/ollama/src external/podman/src || true
fi

mkdir -p bin
engine="bin/bashy-podman-test${ext}"
"$BASHY_EXE" go build -trimpath -tags "$tags" -o "$engine" ./cmd/bashy

machine_list="bin/bashy-podman-machine-list.txt"
info_log="bin/bashy-podman-info.txt"
"$engine" podman machine list >"$machine_list"
if "$engine" podman info >"$info_log" 2>&1; then
  "$engine" podman run --rm docker.io/library/alpine:3.20 sh -c 'echo bashy-podman-ok'
else
  if grep -Eiq 'wsl2 setup|reboot windows|automatic setup failed|cannot connect to podman|no podman found|not ready|hyper-?v|elevat|administrator' "$info_log"; then
    echo "SKIP: bashy podman substrate is not ready on this host"
    sed -n '1,80p' "$info_log"
    exit 0
  fi
  cat "$info_log"
  exit 1
fi
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
BASHY_EXE="${BASHY:-bashy}"
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}'"
for plat in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os=${plat%/*}; arch=${plat#*/}; ext=""
  [ "$os" = windows ] && ext=.exe
  for name in bash bashy; do
    out="bin/dist/${name}-${os}-${arch}${ext}"
    echo "building $out..."
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      "$BASHY_EXE" go build -trimpath -ldflags "$LDFLAGS" -o "$out" "./cmd/${name}"
  done
done
```

### test-bash-data
Ensure the optional GNU Bash 5.3 fixture data is present. This is external GPL
test data, not bashy source and not a build/runtime dependency. bashy does not
vendor it and does not hard-code a default download URL. Set
`BASH53_TESTDATA_REPO` to a public GPL-compatible testdata repo when a runner
needs to hydrate the suite; the target clones it into the gitignored
`external/bash-5.3` directory on first use and pulls with `--ff-only` when it is
already a git checkout. Existing non-git fixture trees are accepted for local
development, but missing fixtures fail loudly.
Effects: write, net

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
dir=external/bash-5.3
repo="${BASH53_TESTDATA_REPO:-}"
if [ -d "$dir/.git" ]; then
  "$BASHY_EXE" git -C "$dir" config core.autocrlf false
  "$BASHY_EXE" git -C "$dir" reset --hard HEAD
  "$BASHY_EXE" git -C "$dir" -c core.autocrlf=false pull --ff-only
  "$BASHY_EXE" git -C "$dir" -c core.autocrlf=false checkout -f HEAD
elif [ -n "$repo" ]; then
  if [ -e "$dir" ] && [ ! -d "$dir/tests" ]; then
    echo "test-bash-data: $dir exists but does not contain tests" >&2
    exit 2
  fi
  if [ -e "$dir" ] && [ ! -d "$dir/.git" ]; then
    echo "test-bash-data: $dir exists but is not a git checkout; remove it or leave BASH53_TESTDATA_REPO unset" >&2
    exit 2
  fi
  "$BASHY_EXE" mkdir -p external
  "$BASHY_EXE" git -c core.autocrlf=false clone "$repo" "$dir"
  "$BASHY_EXE" git -C "$dir" config core.autocrlf false
  "$BASHY_EXE" git -C "$dir" -c core.autocrlf=false checkout -f HEAD
elif [ -d "$dir/tests" ]; then
  :
else
  echo "test-bash-data: missing $dir/tests; set BASH53_TESTDATA_REPO to a git testdata repo" >&2
  exit 2
fi
```

### test-bash
**GNU Bash 5.3 compatibility test** — the canonical conformance gate. Runs
the externally supplied GNU Bash 5.3 GPL test suite against the freshly built
`bin/bash` (the pure drop-in); the headline is the PASS-count three-tuple
(currently **86/86**, 0 fail, 0 skip). The harness is bashy-native, not
make-based: it uses `"$BASHY" go run ./tools/bash53suite`, so it works on hosts
where bashy provides the Go toolchain and no system make is installed. Use
`TESTS="comsub varenv"` for a subset, or `CHUNK=1/4` to run one deterministic
distributed shard.
Requires: build, test-bash-data
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
goos="$("$BASHY_EXE" go env GOOS)"
ext=""
[ "$goos" = windows ] && ext=.exe
"$BASHY_EXE" go run ./tools/bash53suite -tests-dir external/bash-5.3/tests -bash "bin/bash${ext}"
```

### test-bash-list
List the GNU Bash 5.3 fixtures known to the bashy-native harness.
Requires: test-bash-data
Effects: read

```bash
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go run ./tools/bash53suite -tests-dir external/bash-5.3/tests -list
```

### test-bash-chunk
Run one GNU Bash 5.3 distributed chunk. Set `CHUNK=I/N`, for example
`CHUNK=2/6 bashy dag test-bash-chunk`; optional `TESTS="..."` still narrows the
fixture set before chunking.
Requires: build, test-bash-data
Effects: write

```bash
set -e
: "${CHUNK:?set CHUNK=I/N, for example CHUNK=1/4}"
BASHY_EXE="${BASHY:-bashy}"
goos="$("$BASHY_EXE" go env GOOS)"
ext=""
[ "$goos" = windows ] && ext=.exe
"$BASHY_EXE" go run ./tools/bash53suite -tests-dir external/bash-5.3/tests -bash "bin/bash${ext}"
```

### test-bash-container
Run the GNU Bash 5.3 conformance gate in a Linux container through `bashy
podman`. This is the cross-platform release lane for Windows hosts: bashy
cross-builds the pure `cmd/bash` testee and the bashy-native harness for Linux,
then runs the external GPL fixture data inside a Linux userspace with `gcc`
available for Bash's small helper programs. Set `BASH53_TESTDATA_REPO` to
hydrate the gitignored fixture tree; set `TESTS="..."` or `CHUNK=I/N` for
subset/distributed runs.
Requires: test-bash-data, test-podman
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
host_goos="$("$BASHY_EXE" go env GOOS)"
host_goarch="$("$BASHY_EXE" go env GOARCH)"
ext=""
[ "$host_goos" = windows ] && ext=.exe
engine="bin/bashy-podman-test${ext}"
testee_dir="bin/bash-linux-${host_goarch}"
testee="${testee_dir}/bash"
harness="bin/bash53suite-linux-${host_goarch}"
[ -f "$testee_dir" ] && rm -f "$testee_dir"
mkdir -p "$testee_dir"
GOOS=linux GOARCH="$host_goarch" CGO_ENABLED=0 "$BASHY_EXE" go build -trimpath -o "$testee" ./cmd/bash
GOOS=linux GOARCH="$host_goarch" CGO_ENABLED=0 "$BASHY_EXE" go build -trimpath -o "$harness" ./tools/bash53suite
repo="$("$BASHY_EXE" pwd)"
"$engine" podman run --rm \
  -v "$repo:/work" \
  -w /work \
  -e TESTS="${TESTS:-}" \
  -e CHUNK="${CHUNK:-}" \
  docker.io/library/gcc:14-bookworm \
  "./$harness" -tests-dir external/bash-5.3/tests -bash "./$testee"
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
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go mod tidy
"$BASHY_EXE" go fmt ./...
"$BASHY_EXE" go vet ./...
```

### clean
Remove built binaries.
Effects: destroy

```bash
rm -rf bin
```
