#!/usr/bin/env bash
# Release-time view of the siblings bashy pins in go.mod (no clones, no pin
# file): download the pinned outpost and yoke modules and link ../outpost and
# ../yoke to their read-only module directories, so the release steps that
# read those trees (goreleaser's outpost build, the outpost license, the
# seed-band check) see exactly the pinned source. Exports OUTPOST_COMMIT (the
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
outpost_dir=$(printf '%s' "$outpost" | field Dir)
outpost_commit=$(printf '%s' "$outpost" | field Origin Hash)
if [[ -z $outpost_commit ]]; then
  # A cache hit carries no Origin; expand the pseudo-version's revision.
  rev=$(printf '%s' "$outpost" | field Version | sed -n 's/.*-\([0-9a-f]\{12\}\)$/\1/p')
  auth=(); [[ -n ${GITHUB_TOKEN:-} ]] && auth=(-H "Authorization: Bearer $GITHUB_TOKEN")
  outpost_commit=$(curl -fsSL "${auth[@]}" "https://api.github.com/repos/qiangli/outpost/commits/$rev" | field sha)
fi
yoke_dir=$(printf '%s' "$yoke" | field Dir)
[[ $outpost_commit =~ ^[0-9a-f]{40}$ ]] || { echo "resolve-pinned-siblings: no outpost commit" >&2; exit 1; }
shell_runtime=$(sed -n 's|^replace mvdan.cc/sh/v3 => github.com/qiangli/sh/v3 ||p' go.mod)
[[ -n $shell_runtime ]] || { echo "resolve-pinned-siblings: no sh fork replace in go.mod" >&2; exit 1; }
for pair in "outpost:$outpost_dir" "yoke:$yoke_dir"; do
  name=${pair%%:*}; dir=${pair#*:}
  if [[ -e ../$name && ! -L ../$name ]]; then
    echo "resolve-pinned-siblings: ../$name exists and is not a link; leaving it" >&2
  else
    ln -sfn "$dir" "../$name"
  fi
done
if [[ -n ${GITHUB_ENV:-} ]]; then
  { echo "OUTPOST_COMMIT=$outpost_commit"; echo "SHELL_RUNTIME_COMMIT=$shell_runtime"; echo "SHELL_RUNTIME_COMMIT_TIME="; } >> "$GITHUB_ENV"
fi
echo "OUTPOST_COMMIT=$outpost_commit"
echo "SHELL_RUNTIME_COMMIT=$shell_runtime"
