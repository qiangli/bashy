#!/usr/bin/env bash
# Verify release build metadata for the target-specific embedded runner.
set -euo pipefail
[[ $# == 3 ]] || { echo "usage: $0 BINARY OS ARCH" >&2; exit 2; }
binary=$1 os=$2 arch=$3
[[ -f $binary ]] || { echo "verify-runner-release: missing $binary" >&2; exit 1; }
info=$(go version -m "$binary")
setting() { sed -n "s/^[[:space:]]*build[[:space:]]*$1=//p" <<<"$info"; }
[[ $(setting GOOS) == "$os" && $(setting GOARCH) == "$arch" ]] || {
  echo "verify-runner-release: build target mismatch for $binary" >&2; exit 1;
}
root=$(cd "$(dirname "$0")/.." && pwd)
yoke_version=$(GOWORK=off go mod edit -json "$root/go.mod" | python3 -c '
import json,sys
for req in json.load(sys.stdin)["Require"]:
    if req["Path"] == "github.com/qiangli/yoke": print(req["Version"]); break
')
grep -Fq $'dep\tgithub.com/qiangli/yoke\t'"$yoke_version" <<<"$info" || {
  echo "verify-runner-release: artifact does not record yoke pin $yoke_version" >&2; exit 1;
}
tags=$(setting -tags)
case "$os" in
  linux|darwin)
    [[ ,$tags, == *,embed_runner,* ]] || { echo "verify-runner-release: $os/$arch lacks embed_runner" >&2; exit 1; }
    echo "runner release: PASS $os/$arch yoke=$yoke_version tags=$tags"
    ;;
  windows)
    [[ ,$tags, != *,embed_runner,* ]] || { echo "verify-runner-release: Windows carries an unverified runner" >&2; exit 1; }
    echo "runner release: INCOMPLETE windows/$arch (no native pinned runner); refusing to package" >&2
    exit 1
    ;;
  *) echo "verify-runner-release: unsupported target $os/$arch" >&2; exit 1 ;;
esac
