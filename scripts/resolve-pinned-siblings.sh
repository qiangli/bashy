#!/usr/bin/env bash
# CI view of the siblings bashy pins in go.mod (no pin file): link ../outpost
# to the pinned outpost module directory (read-only: goreleaser's outpost
# build, the outpost license) and clone ../yoke at the pinned yoke commit
# (writable: the meet SPA freshness check builds inside pkg/meet/web; the
# seed-band check reads it). Exports OUTPOST_COMMIT (the
# full commit, for the fleet webhook) and SHELL_RUNTIME_COMMIT (the sh fork
# version on go.mod's replace line) to $GITHUB_ENV when set, and prints them.
set -euo pipefail
cd "$(dirname "$0")/.."
export GOWORK=off
info() { GOPROXY=direct GOFLAGS=-mod=mod go mod download -json "$1"; }
field() { python3 -c 'import json,sys
v=json.load(sys.stdin)
for k in sys.argv[1:]:
    v=v.get(k) if isinstance(v,dict) else None
print(v or "")' "$@"; }
outpost=$(info github.com/qiangli/outpost)
yoke=$(info github.com/qiangli/yoke)
# commit INFO REPO: the full commit a module version names. A cache hit carries
# no Origin, so expand the pseudo-version's revision through the GitHub API.
commit() {
  local c rev auth=()
  c=$(printf '%s' "$1" | field Origin Hash)
  if [[ -z $c ]]; then
    rev=$(printf '%s' "$1" | field Version | sed -n 's/.*-\([0-9a-f]\{12\}\)$/\1/p')
    [[ -n ${GITHUB_TOKEN:-} ]] && auth=(-H "Authorization: Bearer $GITHUB_TOKEN")
    c=$(curl -fsSL "${auth[@]}" "https://api.github.com/repos/$2/commits/$rev" | field sha)
  fi
  printf '%s' "$c"
}
outpost_dir=$(printf '%s' "$outpost" | field Dir)
outpost_commit=$(commit "$outpost" qiangli/outpost)
yoke_commit=$(commit "$yoke" qiangli/yoke)
[[ $yoke_commit =~ ^[0-9a-f]{40}$ ]] || { echo "resolve-pinned-siblings: no yoke commit" >&2; exit 1; }
[[ $outpost_commit =~ ^[0-9a-f]{40}$ ]] || { echo "resolve-pinned-siblings: no outpost commit" >&2; exit 1; }
shell_runtime=$(sed -n 's|^replace mvdan.cc/sh/v3 => github.com/qiangli/sh/v3 ||p' go.mod)
[[ -n $shell_runtime ]] || { echo "resolve-pinned-siblings: no sh fork replace in go.mod" >&2; exit 1; }
if [[ -e ../outpost && ! -L ../outpost ]]; then
  echo "resolve-pinned-siblings: ../outpost exists and is not a link; leaving it" >&2
else
  ln -sfn "$outpost_dir" ../outpost
fi
if [[ -e ../yoke ]]; then
  echo "resolve-pinned-siblings: ../yoke exists; leaving it" >&2
else
  git clone --quiet --filter=blob:none https://github.com/qiangli/yoke.git ../yoke
  git -C ../yoke checkout --quiet "$yoke_commit"
fi
if [[ -n ${GITHUB_ENV:-} ]]; then
  { echo "OUTPOST_COMMIT=$outpost_commit"; echo "SHELL_RUNTIME_COMMIT=$shell_runtime"; echo "SHELL_RUNTIME_COMMIT_TIME="; } >> "$GITHUB_ENV"
fi
echo "OUTPOST_COMMIT=$outpost_commit"
echo "SHELL_RUNTIME_COMMIT=$shell_runtime"
