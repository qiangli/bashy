#!/usr/bin/env bash
# Build or stage the embedded inference runner from the exact yoke go.mod pin.
# All checkouts, module downloads, and Go build cache writes stay below WORKDIR.
set -euo pipefail

usage() {
  echo "usage: $0 build OS ARCH OUTDIR WORKDIR | workspace OS ARCH RUNNER_GZ MANIFEST WORKDIR" >&2
  exit 2
}
[[ $# -ge 1 ]] || usage
mode=$1; shift
root=$(cd "$(dirname "$0")/.." && pwd)

yoke_version=$(GOWORK=off go mod edit -json "$root/go.mod" | python3 -c '
import json,sys
for req in json.load(sys.stdin)["Require"]:
    if req["Path"] == "github.com/qiangli/yoke": print(req["Version"]); break
')
[[ $yoke_version =~ ^v0\.0\.0-[0-9]{14}-[0-9a-f]{12}$ ]] || {
  echo "pinned-yoke-runner: yoke must be an exact pseudo-version, got $yoke_version" >&2
  exit 1
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}';
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

checkout() {
  local work=$1 meta commit
  mkdir -p "$work/gomodcache" "$work/gocache"
  meta=$(GOWORK=off GOMODCACHE="$work/gomodcache" GOCACHE="$work/gocache" \
    go mod download -json "github.com/qiangli/yoke@$yoke_version")
  commit=$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("Origin",{}).get("Hash",""))' <<<"$meta")
  [[ $commit =~ ^[0-9a-f]{40}$ && ${commit:0:12} == "${yoke_version##*-}" ]] || {
    echo "pinned-yoke-runner: cannot resolve full commit for $yoke_version" >&2
    exit 1
  }
  git clone --quiet --filter=blob:none --no-checkout https://github.com/qiangli/yoke.git "$work/yoke"
  git -C "$work/yoke" checkout --quiet --detach "$commit"
  [[ $(git -C "$work/yoke" rev-parse HEAD) == "$commit" ]] || exit 1
  [[ -z $(git -C "$work/yoke" status --porcelain) ]] || {
    echo "pinned-yoke-runner: fresh yoke checkout is dirty" >&2; exit 1;
  }
  printf '%s\n' "$commit"
}

case "$mode" in
  build)
    [[ $# == 4 ]] || usage
    os=$1 arch=$2 outdir=$3 work=$4
    [[ ! -e $work ]] || { echo "pinned-yoke-runner: workdir already exists: $work" >&2; exit 1; }
    mkdir -p "$work" "$outdir"
    commit=$(checkout "$work")
    host_os=$(go env GOOS); host_arch=$(go env GOARCH)
    [[ $os == "$host_os" && $arch == "$host_arch" ]] || {
      echo "pinned-yoke-runner: native builder required: target $os/$arch, host $host_os/$host_arch" >&2
      exit 1
    }
    case "$os/$arch" in linux/amd64|linux/arm64|darwin/amd64|darwin/arm64) ;; *)
      echo "pinned-yoke-runner: unsupported target $os/$arch" >&2; exit 1;; esac
    git -C "$work/yoke" submodule update --init --depth 1 external/ollama/src
    ollama_commit=$(git -C "$work/yoke/external/ollama/src" rev-parse HEAD)
    expected_ollama=$(git -C "$work/yoke" rev-parse :external/ollama/src)
    [[ $ollama_commit == "$expected_ollama" ]] || { echo "pinned-yoke-runner: ollama gitlink mismatch" >&2; exit 1; }
    [[ ! -e $work/yoke/external/ollama/runner_embed/ycode-runner.gz ]] || {
      echo "pinned-yoke-runner: stale gzip present before native build" >&2; exit 1;
    }
    export GOMODCACHE="$work/gomodcache" GOCACHE="$work/gocache"
    if [[ $os/$arch == darwin/amd64 ]]; then
      # yoke 98dd834f's source-pinned helper predates its Intel allow-list.
      # Run the identical pinned generation/build recipe for the native Intel lane.
      builddir="$work/build"; mkdir -p "$builddir"
      (cd "$work/yoke/external/ollama/src"; export GOWORK=off; go generate ./ml/... ./x/...; CGO_ENABLED=1 go build -trimpath -o "$builddir/ycode-runner" ./cmd/runner/)
      gzip -n -9 -c "$builddir/ycode-runner" > "$work/yoke/external/ollama/runner_embed/ycode-runner.gz"
      gzip -t "$work/yoke/external/ollama/runner_embed/ycode-runner.gz"
    else
      RUNNER_TARGET_OS="$os" RUNNER_TARGET_ARCH="$arch" "$work/yoke/scripts/build-runner-thin.sh"
    fi
    source_gz="$work/yoke/external/ollama/runner_embed/ycode-runner.gz"
    [[ -s $source_gz ]] && gzip -t "$source_gz"
    gzip -dc "$source_gz" > "$work/runner"
    runner_sha=$(sha256_file "$work/runner")
    gzip_sha=$(sha256_file "$source_gz")
    cp "$source_gz" "$outdir/runner-$os-$arch.gz"
    python3 - "$outdir/runner-$os-$arch.json" "$os" "$arch" "$yoke_version" "$commit" "$ollama_commit" "$gzip_sha" "$runner_sha" <<'PY'
import json, pathlib, sys
path, os_name, arch, version, commit, ollama, gz_sha, runner_sha = sys.argv[1:]
pathlib.Path(path).write_text(json.dumps({
    "schema": "bashy-pinned-runner-v1", "target": f"{os_name}/{arch}",
    "yoke_version": version, "yoke_commit": commit, "ollama_commit": ollama,
    "gzip_sha256": gz_sha, "runner_sha256": runner_sha,
}, sort_keys=True, indent=2) + "\n")
PY
    ;;
  workspace)
    [[ $# == 5 ]] || usage
    os=$1 arch=$2 runner_gz=$3 manifest=$4 work=$5
    [[ -s $runner_gz && -s $manifest ]] || { echo "pinned-yoke-runner: missing runner input" >&2; exit 1; }
    [[ ! -e $work ]] || { echo "pinned-yoke-runner: workdir already exists: $work" >&2; exit 1; }
    mkdir -p "$work"
    commit=$(checkout "$work")
    python3 - "$manifest" "$os/$arch" "$yoke_version" "$commit" "$(sha256_file "$runner_gz")" <<'PY'
import json, sys
p, target, version, commit, digest = sys.argv[1:]
d=json.load(open(p))
want={"schema":"bashy-pinned-runner-v1","target":target,"yoke_version":version,"yoke_commit":commit,"gzip_sha256":digest}
bad={k:(d.get(k),v) for k,v in want.items() if d.get(k)!=v}
if bad: raise SystemExit(f"runner manifest mismatch: {bad}")
PY
    dst="$work/yoke/external/ollama/runner_embed/ycode-runner.gz"
    [[ ! -e $dst ]] || { echo "pinned-yoke-runner: stale gzip in clean checkout" >&2; exit 1; }
    cp "$runner_gz" "$dst"; gzip -t "$dst"
    (cd "$work" && GOWORK=off go work init "$root")
    GOWORK="$work/go.work" go work edit -replace "github.com/qiangli/yoke=$work/yoke"
    printf 'GOWORK=%s\nGOMODCACHE=%s\nGOCACHE=%s\nYOKE_COMMIT=%s\nRUNNER_MANIFEST=%s\n' \
      "$work/go.work" "$work/gomodcache" "$work/gocache" "$commit" "$manifest"
    ;;
  *) usage ;;
esac
