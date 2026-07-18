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
BUILD_ID=""
if [ -e .git ] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  BUILD_ID=$(git describe --tags --exact-match HEAD 2>/dev/null || git rev-parse --short=7 HEAD 2>/dev/null || true)
  if [ -n "$BUILD_ID" ]; then
    if ! git diff --quiet --ignore-submodules -- 2>/dev/null || ! git diff --cached --quiet --ignore-submodules -- 2>/dev/null; then
      BUILD_ID="${BUILD_ID}-dirty"
    fi
  fi
fi
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}' -X 'github.com/qiangli/bashy/internal/cli.buildID=${BUILD_ID}'"
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
BUILD_ID=""
if [ -e .git ] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  BUILD_ID=$(git describe --tags --exact-match HEAD 2>/dev/null || git rev-parse --short=7 HEAD 2>/dev/null || true)
  if [ -n "$BUILD_ID" ]; then
    if ! git diff --quiet --ignore-submodules -- 2>/dev/null || ! git diff --cached --quiet --ignore-submodules -- 2>/dev/null; then
      BUILD_ID="${BUILD_ID}-dirty"
    fi
  fi
fi
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}' -X 'github.com/qiangli/bashy/internal/cli.buildID=${BUILD_ID}'"
"$BASHY_EXE" go build -trimpath -tags "bashy_engines bashy_obs" -ldflags "$LDFLAGS" -o "bin/bashy${ext}" ./cmd/bashy
```

### install
go install both binaries into GOBIN.
Effects: write

```bash
VERSION="${VERSION:-dev}"
BASHY_EXE="${BASHY:-bashy}"
BUILD_ID=""
if [ -e .git ] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  BUILD_ID=$(git describe --tags --exact-match HEAD 2>/dev/null || git rev-parse --short=7 HEAD 2>/dev/null || true)
  if [ -n "$BUILD_ID" ]; then
    if ! git diff --quiet --ignore-submodules -- 2>/dev/null || ! git diff --cached --quiet --ignore-submodules -- 2>/dev/null; then
      BUILD_ID="${BUILD_ID}-dirty"
    fi
  fi
fi
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}' -X 'github.com/qiangli/bashy/internal/cli.buildID=${BUILD_ID}'"
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
BUILD_ID=""
if [ -e .git ] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  BUILD_ID=$(git describe --tags --exact-match HEAD 2>/dev/null || git rev-parse --short=7 HEAD 2>/dev/null || true)
  if [ -n "$BUILD_ID" ]; then
    if ! git diff --quiet --ignore-submodules -- 2>/dev/null || ! git diff --cached --quiet --ignore-submodules -- 2>/dev/null; then
      BUILD_ID="${BUILD_ID}-dirty"
    fi
  fi
fi
LDFLAGS="-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-${VERSION}' -X 'github.com/qiangli/bashy/internal/cli.buildID=${BUILD_ID}'"
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
elif [ -d "$dir/tests" ]; then
  :
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
else
  echo "test-bash-data: missing $dir/tests; set BASH53_TESTDATA_REPO to a git testdata repo" >&2
  exit 2
fi
```

### test-bash
**GNU Bash 5.3 baremetal compatibility test** — the canonical native host
conformance gate. Runs
the externally supplied GNU Bash 5.3 GPL test suite against the freshly built
`bin/bash` (the pure drop-in); the headline is the PASS-count three-tuple
(currently **86/86**, 0 fail, 0 skip). The harness is bashy-native, not
make-based: it uses `"$BASHY" go run ./tools/bash53suite`, so it works on hosts
where bashy provides the Go toolchain and no system make is installed. Use
`TESTS="comsub varenv"` for a subset, or `CHUNK=1/8` to run one manifest-pinned
distributed shard. Use this lane when host OS behavior is part of the coverage;
use `test-bash-container` / `test-bash-chunks-container` for heterogeneous fleet
throughput.
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

### test-bash-prepare
Prepare the native Bash 5.3 baremetal lane once per host for chunked fanout.
Requires: build, test-bash-data
Effects: write

```bash
:
```

### test-bash-run
Run the native Bash 5.3 harness against an already prepared checkout. This is
the chunk worker leaf used by `test-bash-chunks`; it deliberately has no build
dependency so fanout does not rebuild the same checkout concurrently.
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
`CHUNK=2/8 bashy dag test-bash-chunk`. `N` must match the pinned
`chunk_count` in `chunks.json`; fleet capacity never changes fixture membership.
Requires: build, test-bash-data
Effects: write

```bash
set -e
: "${CHUNK:?set CHUNK=I/N, for example CHUNK=1/8}"
BASHY_EXE="${BASHY:-bashy}"
goos="$("$BASHY_EXE" go env GOOS)"
ext=""
[ "$goos" = windows ] && ext=.exe
"$BASHY_EXE" go run ./tools/bash53suite -tests-dir external/bash-5.3/tests -bash "bin/bash${ext}"
```

### dag-fanout
Run any DAG target as deterministic chunks and aggregate result lines. There
are two modes:

- Chunk-aware mode: target honors `CHUNK=I/N`.
- Item-aware mode: set `ITEM_LIST_TARGET=<target-that-lists-items>` and
  `ITEM_ENV=<env-var-for-space-separated-items>`. Fanout assigns item groups to
  chunks, using `DURATIONS_FILE` when present for greedy duration-balanced
  packing. Set `PLAN_FILE` to reuse a saved chunk assignment; set `REPLAN=1`
  to ignore the saved plan and write a fresh one from current durations.

Set `TARGET=<dag-target>`, `CHUNKS=N`, and optionally
`HOSTS="local puppy=C:/Users/liqiang/poc/dhnt/bashy lj2ivy=/path/to/bashy"` to
spread chunks round-robin across hosts. A host named `local` runs in this
checkout; any other host is invoked through ssh. `host=/path` entries set the
remote checkout path, otherwise the local checkout path is reused. Remote hosts
must already have the source checkout, bashy binary, and any target-specific
substrate ready. Set `PREP_TARGET=<dag-target>` to run a setup target once per
participating host before chunk workers start; use this for builds, container
engine checks, and test-data hydration that must not race per chunk.
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
target="${TARGET:?set TARGET to a chunk-aware DAG target, for example TARGET=test-bash-chunk}"
chunks="${CHUNKS:-12}"
hosts="${HOSTS:-local}"
item_list_target="${ITEM_LIST_TARGET:-}"
item_env="${ITEM_ENV:-ITEMS}"
durations_file="${DURATIONS_FILE:-}"
plan_file="${PLAN_FILE:-}"
replan="${REPLAN:-}"
prep_target="${PREP_TARGET:-}"
case "$chunks" in ''|*[!0-9]*) echo "CHUNKS must be a positive integer" >&2; exit 2;; esac
[ "$chunks" -gt 0 ] || { echo "CHUNKS must be > 0" >&2; exit 2; }
repo="$("$BASHY_EXE" pwd)"
outdir="bin/dag-fanout-${target}"
rm -rf "$outdir"
mkdir -p "$outdir"
if [ -n "$item_list_target" ]; then
  if [ -n "${FANOUT_ITEMS:-}" ]; then
    printf '%s\n' $FANOUT_ITEMS >"$outdir/items"
  else
    "$BASHY_EXE" dag "$item_list_target" | awk 'NF && $1 != "==>" { print $1 }' >"$outdir/items"
  fi
  [ -s "$outdir/items" ] || { echo "dag-fanout: no items from $item_list_target" >&2; exit 2; }
  i=1
  while [ "$i" -le "$chunks" ]; do : >"$outdir/group-$i"; i=$((i + 1)); done
  use_plan=0
  if [ -n "$plan_file" ] && [ -f "$plan_file" ] && [ "$replan" != 1 ]; then
    awk -v chunks="$chunks" -v outdir="$outdir" '
      NR == FNR { wanted[$1] = 1; total++; next }
      $1 ~ /^[0-9]+$/ && $1 >= 1 && $1 <= chunks && wanted[$2] {
        print $2 >> (outdir "/group-" $1)
        if (!assigned[$2]) assigned_count++
        assigned[$2] = 1
      }
      END {
        for (item in wanted) if (!assigned[item]) missing++
        if (missing || assigned_count != total) exit 1
      }
    ' "$outdir/items" "$plan_file" && use_plan=1 || use_plan=0
  fi
  if [ "$use_plan" != 1 ]; then
    i=1
    while [ "$i" -le "$chunks" ]; do : >"$outdir/group-$i"; i=$((i + 1)); done
  while IFS= read -r item; do
    dur=1
    if [ -n "$durations_file" ] && [ -f "$durations_file" ]; then
      found=$(awk -v item="$item" '$1 == item { print $2; found=1; exit } END { if (!found) print "" }' "$durations_file")
      [ -n "$found" ] && dur="$found"
    fi
    printf '%s\t%s\n' "$dur" "$item"
  done <"$outdir/items" | sort -nr >"$outdir/items.weighted"
  awk -v chunks="$chunks" -v outdir="$outdir" '
    BEGIN {
      for (i = 1; i <= chunks; i++) load[i] = 0
    }
    {
      dur = $1 + 0
      item = $2
      best = 1
      for (i = 2; i <= chunks; i++) if (load[i] < load[best]) best = i
      print item >> (outdir "/group-" best)
      load[best] += dur
    }
  ' "$outdir/items.weighted"
  fi
  : >"$outdir/plan.tsv"
  i=1
  while [ "$i" -le "$chunks" ]; do
    awk -v chunk="$i" '{ print chunk "\t" $1 }' "$outdir/group-$i" >>"$outdir/plan.tsv"
    i=$((i + 1))
  done
  if [ -n "$plan_file" ] && [ "$replan" = 1 ]; then
    mkdir -p "$(dirname "$plan_file")"
    cp "$outdir/plan.tsv" "$plan_file"
  fi
fi
set -- $hosts
host_count=$#
[ "$host_count" -gt 0 ] || { echo "HOSTS must not be empty" >&2; exit 2; }
if [ -n "$prep_target" ]; then
  prep_i=1
  for host in "$@"; do
    remote_dir="$repo"
    case "$host" in
      *=*) remote="${host%%=*}"; remote_dir="${host#*=}" ;;
      *) remote="$host" ;;
    esac
    echo "dag-fanout: preparing $remote with $prep_target"
    if [ "$remote" = local ]; then
      "$BASHY_EXE" dag "$prep_target" >"$outdir/prep-$prep_i.out" 2>&1 || {
        cat "$outdir/prep-$prep_i.out"
        exit 1
      }
    else
      ssh "$remote" "cd '$remote_dir' && if [ -x ./bashy ]; then b=./bashy; elif [ -x ./bin/bashy.exe ]; then b=./bin/bashy.exe; else b=./bin/bashy; fi; BASH53_TESTDATA_REPO='${BASH53_TESTDATA_REPO:-}' \"\$b\" dag '$prep_target'" >"$outdir/prep-$prep_i.out" 2>&1 || {
        cat "$outdir/prep-$prep_i.out"
        exit 1
      }
    fi
    cat "$outdir/prep-$prep_i.out"
    prep_i=$((prep_i + 1))
  done
fi
i=1
while [ "$i" -le "$chunks" ]; do
  idx=$(( (i - 1) % host_count + 1 ))
  eval "host=\${$idx}"
  (
    remote_dir="$repo"
    case "$host" in
      *=*) remote="${host%%=*}"; remote_dir="${host#*=}" ;;
      *) remote="$host" ;;
    esac
    if [ "$remote" = local ]; then
      if [ -n "$item_list_target" ]; then
        items=$(tr '\n' ' ' <"$outdir/group-$i")
        export "$item_env=$items"
        set +e
        "$BASHY_EXE" dag "$target" >"$outdir/chunk-$i.out" 2>&1
        rc=$?
        set -e
      else
        set +e
        CHUNK="$i/$chunks" "$BASHY_EXE" dag "$target" >"$outdir/chunk-$i.out" 2>&1
        rc=$?
        set -e
      fi
    else
      if [ -n "$item_list_target" ]; then
        items=$(tr '\n' ' ' <"$outdir/group-$i")
        set +e
        ssh "$remote" "cd '$remote_dir' && if [ -x ./bashy ]; then b=./bashy; elif [ -x ./bin/bashy.exe ]; then b=./bin/bashy.exe; else b=./bin/bashy; fi; export BASH53_TESTDATA_REPO='${BASH53_TESTDATA_REPO:-}' $item_env='$items'; \"\$b\" dag '$target'" >"$outdir/chunk-$i.out" 2>&1
        rc=$?
        set -e
      else
        set +e
        ssh "$remote" "cd '$remote_dir' && if [ -x ./bashy ]; then b=./bashy; elif [ -x ./bin/bashy.exe ]; then b=./bin/bashy.exe; else b=./bin/bashy; fi; BASH53_TESTDATA_REPO='${BASH53_TESTDATA_REPO:-}' CHUNK='$i/$chunks' \"\$b\" dag '$target'" >"$outdir/chunk-$i.out" 2>&1
        rc=$?
        set -e
      fi
    fi
    echo "$rc" >"$outdir/chunk-$i.status"
  ) &
  i=$((i + 1))
done
wait || true
cat "$outdir"/chunk-*.out
bad=0
for status in "$outdir"/chunk-*.status; do
  [ "$(cat "$status")" = 0 ] || bad=1
done
set +e
awk '
  /^Results:/ {
    p += $2; f += $4; s += $6; t += $8; seen++
  }
  END {
    printf("\nChunk aggregate: %d passed, %d failed, %d skipped, %d timed out (%d chunks)\n", p, f, s, t, seen)
    if (seen == 0 || f != 0 || t != 0) exit 1
  }
' "$outdir"/chunk-*.out
aggregate=$?
set -e
if [ -n "$durations_file" ]; then
  awk '$1 == "DURATION" { print $2 "\t" $3 }' "$outdir"/chunk-*.out >"$outdir/durations.new"
  [ -s "$outdir/durations.new" ] && cp "$outdir/durations.new" "$durations_file"
fi
[ "$aggregate" -eq 0 ] || exit "$aggregate"
[ "$bad" -eq 0 ]
```

### dag-fanout-tune
Run a generic fanout target repeatedly until chunking settles or `MAX_ROUNDS`
is reached. Each round replans from the duration profile left by the previous
round, records wall time, and saves the best assignment to `PLAN_FILE` for
normal future runs. Set `SETTLE_ROUNDS=N` to stop after N rounds without a new
best wall time.
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
target="${TARGET:?set TARGET to a DAG target, for example TARGET=test-bash}"
chunks="${CHUNKS:-12}"
max_rounds="${MAX_ROUNDS:-3}"
settle_rounds="${SETTLE_ROUNDS:-2}"
plan_file="${PLAN_FILE:-bin/dag-fanout-${target}.plan.tsv}"
outdir="bin/dag-fanout-tune-${target}"
case "$max_rounds" in ''|*[!0-9]*) echo "MAX_ROUNDS must be a positive integer" >&2; exit 2;; esac
case "$settle_rounds" in ''|*[!0-9]*) echo "SETTLE_ROUNDS must be a positive integer" >&2; exit 2;; esac
[ "$max_rounds" -gt 0 ] || { echo "MAX_ROUNDS must be > 0" >&2; exit 2; }
[ "$settle_rounds" -gt 0 ] || { echo "SETTLE_ROUNDS must be > 0" >&2; exit 2; }
rm -rf "$outdir"
mkdir -p "$outdir"
best_wall=""
best_round=0
quiet_rounds=0
overall=0
round=1
while [ "$round" -le "$max_rounds" ]; do
  start=$(date +%s)
  round_plan="$outdir/round-$round.plan.tsv"
  set +e
  TARGET="$target" CHUNKS="$chunks" HOSTS="${HOSTS:-local}" \
  PREP_TARGET="${PREP_TARGET:-}" \
  ITEM_LIST_TARGET="${ITEM_LIST_TARGET:-}" ITEM_ENV="${ITEM_ENV:-ITEMS}" \
  FANOUT_ITEMS="${FANOUT_ITEMS:-}" DURATIONS_FILE="${DURATIONS_FILE:-}" \
  PLAN_FILE="$round_plan" REPLAN=1 "$BASHY_EXE" dag dag-fanout
  rc=$?
  set -e
  end=$(date +%s)
  wall=$((end - start))
  echo "$round	$wall	$rc" >>"$outdir/rounds.tsv"
  cp -R "bin/dag-fanout-${target}" "$outdir/round-$round"
  improved=0
  if [ -z "$best_wall" ]; then
    improved=1
  elif awk -v wall="$wall" -v best="$best_wall" 'BEGIN { exit !(wall < best) }'; then
    improved=1
  fi
  if [ "$improved" = 1 ]; then
    best_wall="$wall"
    best_round="$round"
    quiet_rounds=0
    cp "$round_plan" "$outdir/best.plan.tsv"
  else
    quiet_rounds=$((quiet_rounds + 1))
  fi
  [ "$rc" = 0 ] || overall="$rc"
  echo "Tune round $round: ${wall}s (exit $rc, best round $best_round at ${best_wall}s)"
  [ "$quiet_rounds" -lt "$settle_rounds" ] || break
  round=$((round + 1))
done
if [ -f "$outdir/best.plan.tsv" ]; then
  mkdir -p "$(dirname "$plan_file")"
  cp "$outdir/best.plan.tsv" "$plan_file"
fi
echo "Tune best: round $best_round at ${best_wall}s; saved $plan_file"
exit "$overall"
```

### test-bash-chunks
Run the GNU Bash 5.3 suite through the generic DAG fanout target on the native
host OS. This is the baremetal lane: use it when platform behavior is part of
the test. For heterogeneous fleet capacity, prefer `test-bash-chunks-container`.
Chunk membership and chunk count come from committed `chunks.json`; `HOSTS`
controls only how many manifest chunks run concurrently and where they are
placed. `BASH53_TIMEOUT=55s` can be used for exploratory runs; the canonical
single-process gate keeps its default 60s timeout. Set `HOSTS="local puppy"`
only when the remote host can run the target noninteractively and the host OS
variance is intentional.
Requires: build, test-bash-data
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
mkdir -p bin
pinned_chunks="$("$BASHY_EXE" go run ./tools/bash53suite -chunk-count)"
if [ -n "${CHUNKS:-}" ] && [ "$CHUNKS" != "$pinned_chunks" ]; then
  echo "test-bash-chunks: CHUNKS=$CHUNKS conflicts with chunks.json chunk_count=$pinned_chunks" >&2
  exit 2
fi
TARGET="${TARGET:-test-bash-run}" \
PREP_TARGET="${PREP_TARGET:-test-bash-prepare}" \
CHUNKS="$pinned_chunks" \
"$BASHY_EXE" dag dag-fanout
```

### test-bash-fleet-check
Check that the standard distributed Bash 5.3 test fleet is reachable and that
each remote checkout exposes a usable bashy binary. Override remote paths with
`NOVICORTEX_DIR=...`, `PUPPY_DIR=...`, or `LJ2IVY_DIR=...`; override the full
fleet with `HOSTS=...`.
Requires: test-bash-fleet-prepare
Effects: read, net

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
repo="$("$BASHY_EXE" pwd)"
novicortex_dir="${NOVICORTEX_DIR:-/Users/noviadmin/projects/poc/dhnt/bashy}"
puppy_dir="${PUPPY_DIR:-C:/Users/liqiang/tests/bashy-self/bashy}"
lj2ivy_dir="${LJ2IVY_DIR:-C:/Users/Lern/tests/bashy-self/bashy}"
hosts="${HOSTS:-local novicortex.local=$novicortex_dir puppy=$puppy_dir lj2ivy=$lj2ivy_dir}"
set -- $hosts
for host in "$@"; do
  remote_dir="$repo"
  case "$host" in
    *=*) remote="${host%%=*}"; remote_dir="${host#*=}" ;;
    *) remote="$host" ;;
  esac
  if [ "$remote" = local ]; then
    "$BASHY_EXE" -c 'echo local ok'
  else
    ssh "$remote" "cd '$remote_dir' && if [ -f ./bin/bashy.exe ]; then b=./bin/bashy.exe; elif [ -f ./bin/bashy ]; then b=./bin/bashy; else b=./bashy; fi; echo '$remote using' \"\$b\"; \"\$b\" -c 'echo $remote ok'"
  fi
done
```

### test-bash-fleet-prepare
Prepare the standard distributed Bash 5.3 test fleet. `FLEET_REF=latest` or a
version tag like `v0.4.1` installs a released bashy seed where possible.
`FLEET_REF=HEAD`, a branch, or a commit hash builds `bin/bashy` and `bin/bash`
from the remote checkout; source refs are checked out without force-resetting
the worktree. By default, `FLEET_REF` is the local commit hash, so remotes do
not silently test stale checkouts. Override remote paths with
`NOVICORTEX_DIR=...`, `PUPPY_DIR=...`, or `LJ2IVY_DIR=...`; override the full
fleet with `HOSTS=...`.
Requires: build
Effects: write, net

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
repo="$("$BASHY_EXE" pwd)"
fleet_ref="${FLEET_REF:-$(git rev-parse HEAD 2>/dev/null || echo HEAD)}"
novicortex_dir="${NOVICORTEX_DIR:-/Users/noviadmin/projects/poc/dhnt/bashy}"
puppy_dir="${PUPPY_DIR:-C:/Users/liqiang/tests/bashy-self/bashy}"
lj2ivy_dir="${LJ2IVY_DIR:-C:/Users/Lern/tests/bashy-self/bashy}"
hosts="${HOSTS:-local novicortex.local=$novicortex_dir puppy=$puppy_dir lj2ivy=$lj2ivy_dir}"
set -- $hosts
for host in "$@"; do
  remote_dir="$repo"
  case "$host" in
    *=*) remote="${host%%=*}"; remote_dir="${host#*=}" ;;
    *) remote="$host" ;;
  esac
  if [ "$remote" = local ]; then
    "$BASHY_EXE" dag build VERSION="$fleet_ref"
    continue
  fi
  ssh "$remote" "cd '$remote_dir'
    FLEET_REF='$fleet_ref'
    set -e
    ref=\"\${FLEET_REF:-HEAD}\"
    mode=source
    case \"\$ref\" in latest|v[0-9]*) mode=release ;; esac
    ext=
    case \"\$(uname -s 2>/dev/null || echo unknown)\" in Windows*) ext=.exe ;; esac
    mkdir -p bin
    fetch_release_seed() {
      want=\"\$1\"
      os=\"\$(uname -s 2>/dev/null || echo unknown)\"
      arch=\"\$(uname -m 2>/dev/null || echo unknown)\"
      case \"\$os\" in
        Darwin*) os=darwin ;;
        Linux*) os=linux ;;
        Windows*) os=windows ;;
        *) echo \"fleet prepare: unsupported release os \$os\" >&2; return 1 ;;
      esac
      case \"\$arch\" in
        x86_64|amd64) arch=amd64 ;;
        arm64|aarch64) arch=arm64 ;;
        *) echo \"fleet prepare: unsupported release arch \$arch\" >&2; return 1 ;;
      esac
      suffix=tar.gz
      [ \"\$os\" = windows ] && suffix=zip
      archive=\"bin/bashy-release.\$suffix\"
      if [ \"\$want\" = latest ]; then
        url=\"https://github.com/qiangli/bashy/releases/latest/download/bashy-\$os-\$arch.\$suffix\"
      else
        url=\"https://github.com/qiangli/bashy/releases/download/\$want/bashy-\$os-\$arch.\$suffix\"
      fi
      command -v curl >/dev/null 2>&1 || return 1
      command -v tar >/dev/null 2>&1 || return 1
      curl -fsSL -o \"\$archive\" \"\$url\"
      tar -xf \"\$archive\" -C bin
      chmod +x \"bin/bashy\$ext\" 2>/dev/null || true
      [ -f \"bin/bashy\$ext\" ]
    }
    fix_windows_ext() {
      [ -n \"\$ext\" ] || return 0
      [ -f bin/bashy ] && cp bin/bashy \"bin/bashy\$ext\"
      [ -f bin/bash ] && cp bin/bash \"bin/bash\$ext\"
      chmod +x \"bin/bashy\$ext\" \"bin/bash\$ext\" 2>/dev/null || true
    }
    sync_sibling() {
      name=\"\$1\"
      sha=\"\$2\"
      dir=\"../\$name\"
      case \"\$name\" in
        sh) url=https://github.com/qiangli/sh.git ;;
        coreutils) url=https://github.com/qiangli/coreutils.git ;;
        readline) url=https://github.com/qiangli/readline.git ;;
        *) return 0 ;;
      esac
      seed_cmd=
      if [ -n \"\$seed\" ]; then
        case \"\$seed\" in
          ./*) seed_cmd=\"\$root/\${seed#./}\" ;;
          *) seed_cmd=\"\$seed\" ;;
        esac
      fi
      if [ -d \"\$dir/.git\" ]; then
        if command -v git >/dev/null 2>&1; then
          (cd \"\$dir\" && git fetch origin --quiet && git checkout \"\$sha\")
        elif command -v outpost >/dev/null 2>&1; then
          (cd \"\$dir\" && outpost git fetch origin && outpost git reset --hard \"\$sha\")
        elif [ -n \"\$seed_cmd\" ]; then
          (cd \"\$dir\" && \"\$seed_cmd\" git fetch origin --quiet && \"\$seed_cmd\" git checkout \"\$sha\")
        else
          echo \"fleet prepare: cannot update \$name without git, outpost git, or bashy git\" >&2
          exit 127
        fi
      else
        if command -v git >/dev/null 2>&1; then
          git clone \"\$url\" \"\$dir\"
          (cd \"\$dir\" && git checkout \"\$sha\")
        elif command -v outpost >/dev/null 2>&1; then
          outpost git clone \"\$url\" \"\$dir\"
          (cd \"\$dir\" && outpost git reset --hard \"\$sha\")
        elif [ -n \"\$seed_cmd\" ]; then
          \"\$seed_cmd\" git clone \"\$url\" \"\$dir\"
          (cd \"\$dir\" && \"\$seed_cmd\" git checkout \"\$sha\")
        else
          echo \"fleet prepare: cannot clone \$name without git, outpost git, or bashy git\" >&2
          exit 127
        fi
      fi
    }
    sync_siblings() {
      [ -f .sibling-pins ] || return 0
      while IFS='=' read -r name sha; do
        case \"\$name\" in ''|\#*) continue ;; esac
        [ -n \"\$sha\" ] || continue
        sync_sibling \"\$name\" \"\$sha\"
      done <.sibling-pins
    }
    seed=
    for candidate in ./bin/bashy\$ext ./bashy bashy; do
      case \"\$candidate\" in
        ./*) [ -f \"\$candidate\" ] || continue ;;
        *) command -v \"\$candidate\" >/dev/null 2>&1 || continue ;;
      esac
      if \"\$candidate\" --version >/dev/null 2>&1; then
        seed=\"\$candidate\"
        break
      fi
    done
    refresh_latest_seed() {
      if [ -n \"\$seed\" ] && \"\$seed\" self install --version latest \"bin/bashy\$ext\" >/dev/null 2>&1; then
        seed=\"./bin/bashy\$ext\"
        return 0
      fi
      if command -v outpost >/dev/null 2>&1 && outpost bashy --install-dir bin >/dev/null 2>&1; then
        seed=\"./bin/bashy\$ext\"
        return 0
      fi
      if fetch_release_seed latest; then
        seed=\"./bin/bashy\$ext\"
        return 0
      fi
      return 1
    }
    if [ \"\$mode\" = release ]; then
      if [ -n \"\$seed\" ] && \"\$seed\" self install --version \"\$ref\" \"bin/bashy\$ext\" >/dev/null 2>&1; then
        seed=\"./bin/bashy\$ext\"
      elif command -v outpost >/dev/null 2>&1; then
        outpost bashy --install-dir bin >/dev/null
        seed=\"./bin/bashy\$ext\"
      elif fetch_release_seed \"\$ref\"; then
        seed=\"./bin/bashy\$ext\"
      fi
      [ -n \"\$seed\" ] || { echo \"fleet prepare: no released bashy seed for \$ref\" >&2; exit 127; }
      \"\$seed\" --version
      exit 0
    fi
    refresh_latest_seed || true
    if [ \"\$ref\" != HEAD ]; then
      if command -v git >/dev/null 2>&1; then
        git fetch --all --tags --quiet || true
        git checkout \"\$ref\"
      elif [ -n \"\$seed\" ]; then
        \"\$seed\" git fetch --all --tags --quiet || true
        \"\$seed\" git checkout \"\$ref\"
      else
        echo \"fleet prepare: cannot checkout \$ref without git or bashy git\" >&2
        exit 127
      fi
    fi
    root=\"\$(pwd)\"
    sync_siblings
    if [ -n \"\$seed\" ]; then
      if \"\$seed\" self build \"bin/bashy\$ext\" --version \"\$ref\" >/dev/null 2>&1; then
        \"./bin/bashy\$ext\" dag build VERSION=\"\$ref\"
      else
        BASHY=\"\$seed\" \"\$seed\" dag build VERSION=\"\$ref\"
      fi
      fix_windows_ext
    elif command -v go >/dev/null 2>&1; then
      build_id=\"\"
      if [ -e .git ] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        build_id=\$(git describe --tags --exact-match HEAD 2>/dev/null || git rev-parse --short=7 HEAD 2>/dev/null || true)
        if [ -n \"\$build_id\" ]; then
          if ! git diff --quiet --ignore-submodules -- 2>/dev/null || ! git diff --cached --quiet --ignore-submodules -- 2>/dev/null; then
            build_id=\"\$build_id-dirty\"
          fi
        fi
      fi
      LDFLAGS=\"-s -w -X github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-\$ref -X github.com/qiangli/bashy/internal/cli.buildID=\$build_id\"
      go build -trimpath -ldflags \"\$LDFLAGS\" -o \"bin/bashy\$ext\" ./cmd/bashy
      \"./bin/bashy\$ext\" dag build VERSION=\"\$ref\"
      fix_windows_ext
    elif command -v outpost >/dev/null 2>&1; then
      outpost bashy --install-dir bin >/dev/null
      \"./bin/bashy\$ext\" dag build VERSION=\"\$ref\"
      fix_windows_ext
    elif fetch_release_seed latest; then
      \"./bin/bashy\$ext\" dag build VERSION=\"\$ref\"
      fix_windows_ext
    else
      echo \"fleet prepare: no bashy seed, go, outpost, or release download path available\" >&2
      exit 127
    fi"
done
```

### test-bash-chunks-fleet
Run the GNU Bash 5.3 chunked suite through the standard development fleet using
the container-normalized lane. The committed `chunks.json` manifest defines the
8 chunks; the fleet only decides how many of those chunks can run concurrently.

- local dragon checkout
- `novicortex.local`
- `puppy`
- `lj2ivy`

Each remote entry points at that host's bashy checkout. `test-bash-fleet-prepare`
ensures a usable bashy exists first; the remote checkouts must still contain the
source, sibling repos, external Bash 5.3 test data, and a working container
substrate. Override any path with `NOVICORTEX_DIR=...`, `PUPPY_DIR=...`, or
`LJ2IVY_DIR=...`; override the whole fleet with `HOSTS=...`. Use
`TARGET=test-bash` with `dag-fanout` directly when native baremetal behavior is
the coverage goal.
Requires: build, test-bash-data, test-bash-fleet-check
Effects: write, net

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
novicortex_dir="${NOVICORTEX_DIR:-/Users/noviadmin/projects/poc/dhnt/bashy}"
puppy_dir="${PUPPY_DIR:-C:/Users/liqiang/tests/bashy-self/bashy}"
lj2ivy_dir="${LJ2IVY_DIR:-C:/Users/Lern/tests/bashy-self/bashy}"
BASH53_TESTDATA_REPO="${BASH53_TESTDATA_REPO:-https://github.com/qiangli/bash53-testdata}" \
HOSTS="${HOSTS:-local novicortex.local=$novicortex_dir puppy=$puppy_dir lj2ivy=$lj2ivy_dir}" \
"$BASHY_EXE" dag test-bash-chunks-container
```

### test-bash-chunks-tune
Tune GNU Bash 5.3 chunk assignments with bounded repeated fanout runs. Each
round replans from the latest `bin/bash53-durations.tsv`, and the best observed
assignment is saved to `bin/bash53-chunks.plan.tsv` for manual review before
updating committed `chunks.json`. Set `MAX_ROUNDS=5`, `SETTLE_ROUNDS=2`, and
optionally `HOSTS=...`.
Requires: build, test-bash-data
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
mkdir -p bin
TARGET="${TARGET:-test-bash-run}" \
PREP_TARGET="${PREP_TARGET:-test-bash-prepare}" \
ITEM_LIST_TARGET="${ITEM_LIST_TARGET:-test-bash-list}" \
ITEM_ENV="${ITEM_ENV:-TESTS}" \
FANOUT_ITEMS="${FANOUT_ITEMS:-${TESTS:-}}" \
DURATIONS_FILE="${DURATIONS_FILE:-bin/bash53-durations.tsv}" \
PLAN_FILE="${PLAN_FILE:-bin/bash53-chunks.plan.tsv}" \
"$BASHY_EXE" dag dag-fanout-tune
```

### test-bash-container
Run the GNU Bash 5.3 conformance gate in a Linux container through `bashy
podman`. This is the cross-platform release lane for Windows hosts: bashy
cross-builds the pure `cmd/bash` testee and the bashy-native harness for Linux,
then runs the external GPL fixture data inside a Linux userspace with `gcc`
available for Bash's small helper programs. Set `BASH53_TESTDATA_REPO` to
hydrate the gitignored fixture tree; set `TESTS="..."` or a manifest-valid
`CHUNK=I/N` for subset/distributed runs. This is the default lane for heterogeneous fleet
throughput because every host contributes a Linux container rather than its
native filesystem/process semantics.
Requires: test-bash-container-prepare
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" dag test-bash-container-run
```

### test-bash-container-prepare
Prepare the GNU Bash 5.3 container-normalized lane once per host. This builds
the host-architecture Linux testee and harness and validates the current
bashy's container surface before fanout starts.
Requires: test-bash-data
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
host_goos="$("$BASHY_EXE" go env GOOS)"
host_goarch="$("$BASHY_EXE" go env GOARCH)"
ext=""
[ "$host_goos" = windows ] && ext=.exe
testee_dir="bin/bash-linux-${host_goarch}"
testee="${testee_dir}/bash"
harness="bin/bash53suite-linux-${host_goarch}"
[ -f "$testee_dir" ] && rm -f "$testee_dir"
mkdir -p "$testee_dir"
GOOS=linux GOARCH="$host_goarch" CGO_ENABLED=0 "$BASHY_EXE" go build -trimpath -o "$testee" ./cmd/bash
GOOS=linux GOARCH="$host_goarch" CGO_ENABLED=0 "$BASHY_EXE" go build -trimpath -o "$harness" ./tools/bash53suite
oci="${BASH53_OCI:-$BASHY_EXE podman}"
$oci info >/dev/null
```

### test-bash-container-run
Run the GNU Bash 5.3 container harness against an already prepared checkout.
This is the chunk worker leaf used by `test-bash-chunks-container`.
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
host_goos="$("$BASHY_EXE" go env GOOS)"
host_goarch="$("$BASHY_EXE" go env GOARCH)"
runner="${BASH53_RUNNER:-container-${host_goarch}-$(printf '%s' "$(hostname)" | sha256sum | cut -c1-8)}"
ext=""
[ "$host_goos" = windows ] && ext=.exe
testee_dir="bin/bash-linux-${host_goarch}"
testee="${testee_dir}/bash"
harness="bin/bash53suite-linux-${host_goarch}"
[ -x "$testee" ] || { echo "missing $testee; run bashy dag test-bash-container-prepare" >&2; exit 2; }
[ -x "$harness" ] || { echo "missing $harness; run bashy dag test-bash-container-prepare" >&2; exit 2; }
repo="$("$BASHY_EXE" pwd)"
tests_root="$(cd external/bash-5.3 && pwd -P)"
oci="${BASH53_OCI:-$BASHY_EXE podman}"
$oci run --rm \
  -v "$repo:/work" \
  -v "$tests_root:/bash53" \
  -w /work \
  -e TESTS="${TESTS:-}" \
  -e CHUNK="${CHUNK:-}" \
  -e BASH53_TIMEOUT="${BASH53_TIMEOUT:-}" \
  -e BASH53_JOBS_TIMEOUT="${BASH53_JOBS_TIMEOUT:-}" \
  -e BASH53_RUNNER="$runner" \
  docker.io/library/gcc:14-bookworm \
  "./$harness" -tests-dir /bash53/tests -bash "./$testee"
```

### test-bash-chunks-container
Run GNU Bash 5.3 compatibility chunks in the container-normalized lane. This is
the fleet-safe target for heterogeneous hosts: each worker runs the same Linux
container substrate, while the native `test-bash-chunks` target remains
available for explicit baremetal OS coverage. Chunk membership and chunk count
come from committed `chunks.json`.
Requires: test-bash-data
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
mkdir -p bin
pinned_chunks="$("$BASHY_EXE" go run ./tools/bash53suite -chunk-count)"
if [ -n "${CHUNKS:-}" ] && [ "$CHUNKS" != "$pinned_chunks" ]; then
  echo "test-bash-chunks-container: CHUNKS=$CHUNKS conflicts with chunks.json chunk_count=$pinned_chunks" >&2
  exit 2
fi
TARGET="${TARGET:-test-bash-container-run}" \
PREP_TARGET="${PREP_TARGET:-test-bash-container-prepare}" \
CHUNKS="$pinned_chunks" \
"$BASHY_EXE" dag dag-fanout
```

### test-bash-chunks-container-tune
Tune GNU Bash 5.3 container-normalized chunk assignments. This should be used
for fleet throughput tuning; use `test-bash-chunks-tune` only for native
baremetal timing.
Requires: test-bash-data
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
mkdir -p bin
TARGET="${TARGET:-test-bash-container-run}" \
PREP_TARGET="${PREP_TARGET:-test-bash-container-prepare}" \
ITEM_LIST_TARGET="${ITEM_LIST_TARGET:-test-bash-list}" \
ITEM_ENV="${ITEM_ENV:-TESTS}" \
FANOUT_ITEMS="${FANOUT_ITEMS:-${TESTS:-}}" \
DURATIONS_FILE="${DURATIONS_FILE:-bin/bash53-container-durations.tsv}" \
PLAN_FILE="${PLAN_FILE:-bin/bash53-container-chunks.plan.tsv}" \
"$BASHY_EXE" dag dag-fanout-tune
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

### yash-list
List chunkable yash POSIX suite files. The GPL yash test checkout remains a
gitignored runtime cache; this target only prints file basenames for DAG fanout.
Effects: write, net

```bash
scripts/yash-posix-suite.sh --list
```

### yash-chunk
Run one chunk of the yash POSIX INFO suite. `YASH_TESTS` is normally supplied by
`dag-fanout`; set it manually for local triage, for example
`YASH_TESTS="alias-p arith-p" bashy dag yash-chunk`.
Effects: write

```bash
scripts/yash-posix-suite.sh "${YASH_OUT:-}"
```

### yash-chunks
Run the yash POSIX INFO suite through generic DAG fanout. Completion is the
success criterion: yash failures are measured and reported by the suite, but
they are not a 0/1 gate for this repo.
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
mkdir -p bin
TARGET="${TARGET:-yash-chunk}" \
ITEM_LIST_TARGET="${ITEM_LIST_TARGET:-yash-list}" \
ITEM_ENV="${ITEM_ENV:-YASH_TESTS}" \
FANOUT_ITEMS="${FANOUT_ITEMS:-${YASH_TESTS:-}}" \
DURATIONS_FILE="${DURATIONS_FILE:-bin/yash-durations.tsv}" \
PLAN_FILE="${PLAN_FILE:-bin/yash-chunks.plan.tsv}" \
"$BASHY_EXE" dag dag-fanout
```

### yash-chunks-tune
Tune yash POSIX INFO suite chunk assignments with bounded repeated fanout runs.
Effects: write

```bash
set -e
BASHY_EXE="${BASHY:-bashy}"
mkdir -p bin
TARGET="${TARGET:-yash-chunk}" \
ITEM_LIST_TARGET="${ITEM_LIST_TARGET:-yash-list}" \
ITEM_ENV="${ITEM_ENV:-YASH_TESTS}" \
FANOUT_ITEMS="${FANOUT_ITEMS:-${YASH_TESTS:-}}" \
DURATIONS_FILE="${DURATIONS_FILE:-bin/yash-durations.tsv}" \
PLAN_FILE="${PLAN_FILE:-bin/yash-chunks.plan.tsv}" \
"$BASHY_EXE" dag dag-fanout-tune
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

### build-bash
Build ONLY `bin/bash` — the pure Bash 5.3 drop-in from cmd/bash (import graph
never includes coreutils; ~5.7 MB). All the conformance harness needs. Uses
`"$BASHY" go build` so the toolchain path is bashy-owned.
Sources: cmd/bash, internal/cli, go.mod, go.sum
Generates: bin/bash
Effects: write

```bash
set -e
mkdir -p bin
BASHY_EXE="${BASHY:-bashy}"
BUILD_ID=""
if [ -e .git ] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  BUILD_ID=$(git rev-parse --short=7 HEAD 2>/dev/null || true)
fi
"$BASHY_EXE" go build -trimpath -ldflags "-s -w -X 'github.com/qiangli/bashy/internal/cli.buildID=$BUILD_ID'" -o bin/bash ./cmd/bash
echo "built bin/bash (pure drop-in)"
```

### build-bashy
Build ONLY `bin/bashy` — the AgentOS shell from cmd/bashy. Set BASHY_TAGS to add
embed tags (embed_podman/…). Lean worker by default.
Sources: cmd/bashy, internal, go.mod, go.sum
Generates: bin/bashy
Effects: write

```bash
set -e
mkdir -p bin
BASHY_EXE="${BASHY:-bashy}"
BUILD_ID=""
if [ -e .git ] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  BUILD_ID=$(git rev-parse --short=7 HEAD 2>/dev/null || true)
fi
tags=""
[ -n "${BASHY_TAGS:-}" ] && tags="-tags ${BASHY_TAGS}"
"$BASHY_EXE" go build -trimpath $tags -ldflags "-s -w -X 'github.com/qiangli/bashy/internal/cli.buildID=$BUILD_ID'" -o bin/bashy ./cmd/bashy
echo "built bin/bashy (AgentOS)"
```

### build-fips
Build both binaries against the Go FIPS 140-3 Cryptographic Module
(GOFIPS140=v1.0.0). Pure-Go, no cgo/BoringCrypto. Use GODEBUG=fips140=on at
runtime (keeps md5sum working), not fips140=only.
Sources: cmd, internal, go.mod, go.sum
Generates: bin/bash, bin/bashy
Effects: write

```bash
set -e
mkdir -p bin
BASHY_EXE="${BASHY:-bashy}"
V="${GOFIPS140_VERSION:-v1.0.0}"
echo "building with the Go FIPS 140-3 module (GOFIPS140=$V) ..."
GOFIPS140="$V" "$BASHY_EXE" go build -trimpath -o bin/bash ./cmd/bash
tags=""; [ -n "${BASHY_TAGS:-}" ] && tags="-tags ${BASHY_TAGS}"
GOFIPS140="$V" "$BASHY_EXE" go build -trimpath $tags -o bin/bashy ./cmd/bashy
echo "built bin/{bash,bashy} in FIPS mode"
```

### test-bash-helpers
Compile the recho/zecho/xcase C helpers the bash-5.3 suite needs (the only place
`cc` is invoked — for test fixtures, not for bashy). Also stubs external/config.h.
Requires: test-bash-data
Sources: external/bash-5.3
Effects: write

```bash
set -e
D="${BASH_TESTS_DIR:-external/bash-5.3/tests}"
cd "$D"
[ -f recho ] || cc -o recho ../support/recho.c 2>/dev/null || true
[ -f zecho ] || cc -o zecho ../support/zecho.c 2>/dev/null || true
[ -f xcase ] || cc -o xcase ../support/xcase.c 2>/dev/null || true
[ -f ../config.h ] || for i in $(seq 1 128); do printf '/* stub config.h line %03d */\n' "$i"; done > ../config.h
echo "test-bash-helpers: recho/zecho/xcase + config.h ready"
```

### test-bash-parallel
The **canonical 86/86 gate** — the bash-5.3 fixture suite fanned across cores via
the single `tools/bash53suite` runner. Same runner as `test-bash`; parallel is
the fast path. JOBS overrides the worker count.
Requires: build-bash, test-bash-helpers
Sources: tools/bash53suite, external/bash-5.3
Effects: write

```bash
set -e
JOBS="${JOBS:-}" BASH_TESTS_DIR="${BASH_TESTS_DIR:-external/bash-5.3/tests}" BASH_TEST_SKIP="${BASH_TEST_SKIP:-}" /bin/bash scripts/test-bash-parallel.sh
```

### test-uutils
uutils/coreutils suite vs the pure-Go multicall (INFO metric; needs cargo).
Sources: scripts/uutils-scoreboard.sh
Effects: write, net

```bash
set -e
scripts/uutils-scoreboard.sh "${UUTILS_OUT:-}"
```

### test-zsh
zsh-own-suite Tier-0 scoreboard (tools/ztst runner; INFO metric, not a gate).
Sources: scripts/zsh-scoreboard.sh
Effects: write

```bash
set -e
scripts/zsh-scoreboard.sh "${ZSH_OUT:-}"
```

### hooks
Install the git pre-push hook (pin-drift guard). Bypass with `git push --no-verify`.
Effects: write

```bash
git config core.hooksPath scripts/hooks
echo "hooks: core.hooksPath -> scripts/hooks (bypass with 'git push --no-verify')"
```

### test-uutils-list
Print the current uutils failure list (INFO; the -list companion to test-uutils).
Sources: scripts/uutils-scoreboard.sh
Effects: read

```bash
set -e
scripts/uutils-scoreboard.sh --list "${UUTILS_OUT:-}"
```

### test-zsh-list
Print the current bashy-specific zsh failure list (INFO; companion to test-zsh).
Sources: scripts/zsh-scoreboard.sh
Effects: read

```bash
set -e
scripts/zsh-scoreboard.sh --list "${ZSH_OUT:-}"
```
