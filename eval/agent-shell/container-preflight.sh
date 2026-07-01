#!/bin/bash
set -euo pipefail

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
work=${1:-"$HOME/tests/bashy-eval"}
ctx="$work/context"
mkdir -p "$ctx" "$work/results"

bashy_bin="$work/bin/bashy-linux-arm64"
image_tag="${BASHY_EVAL_IMAGE:-bashy-agent-shell:bashy-current}"
gnu_image_tag="${GNU_BASH53_EVAL_IMAGE:-bashy-agent-shell:gnu-bash53}"
if [[ ! -x "$bashy_bin" ]]; then
  echo "missing Linux/arm64 bashy binary: $bashy_bin" >&2
  exit 2
fi

cp "$bashy_bin" "$ctx/bashy-linux-arm64"
rm -rf "$ctx/bash-5.3"
mkdir -p "$ctx/bash-5.3"
cp -RL "$repo/external/bash-5.3/." "$ctx/bash-5.3/"

"$repo/bin/bashy" podman build -f "$repo/eval/agent-shell/containers/bashy.Containerfile" \
  -t "$image_tag" "$ctx"

"$repo/bin/bashy" podman build -f "$repo/eval/agent-shell/containers/gnu-bash53.Containerfile" \
  -t "$gnu_image_tag" "$ctx"

"$repo/bin/bashy" podman run --rm "$image_tag" --version | tee "$work/results/bashy-version.txt"
"$repo/bin/bashy" podman run --rm "$gnu_image_tag" --version | sed -n '1p' | tee "$work/results/gnu-bash-version.txt"

"$repo/bin/bashy" podman run --rm "$image_tag" -lc 'echo bashy-lc-ok'
"$repo/bin/bashy" podman run --rm "$gnu_image_tag" -lc 'echo gnu-lc-ok'

cat >"$work/results/container-preflight-summary.txt" <<SUMMARY
container_preflight=pass
bashy_image=$image_tag
gnu_image=$gnu_image_tag
SUMMARY

cat "$work/results/container-preflight-summary.txt"
