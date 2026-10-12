#!/usr/bin/env bash
# check.sh [TOOL...]
#
# Regenerate-and-diff gate: renders every builds/<tool>/tool.env (or the named
# tools) into a scratch dir and fails if builds/<tool>/fork differs, so the
# committed fork trees stay reproducible from their declarations. Also syntax-
# checks the rendered scripts, runs the shared verdict's self-test, and runs
# the shellcheck / actionlint linters when present.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
if (($#)); then
	tools=("$@")
else
	tools=()
	for f in "$here"/*/tool.env; do
		t=${f%/tool.env}
		tools+=("${t##*/}")
	done
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
rc=0
# The one shared strace verdict every fork's no-egress gate sources.
"$here/_shared/egress-verdict-test.sh" || rc=1
for t in "${tools[@]}"; do
	"$here/render.sh" "$t" "$tmp/$t" >/dev/null
	if ! diff -r "$tmp/$t" "$here/$t/fork"; then
		echo "check: $t: fork/ is stale — run builds/render.sh $t" >&2
		rc=1
		continue
	fi
	for s in "$tmp/$t"/bashy-build/*.sh "$tmp/$t"/bashy-build/scripts/*.sh; do
		[[ -f $s ]] || continue
		bash -n "$s" || rc=1
		[[ -x $s ]] || { echo "check: $t: ${s#"$tmp/"} is not executable" >&2; rc=1; }
	done
	if command -v shellcheck >/dev/null; then
		find "$tmp/$t/bashy-build" -name '*.sh' -exec shellcheck -S warning {} + || rc=1
	fi
	if command -v actionlint >/dev/null; then
		(cd "$tmp/$t" && actionlint .github/workflows/bashy-build.yml) || rc=1
	fi
	echo "check: $t: fork/ reproduces from tool.env"
done
exit "$rc"
