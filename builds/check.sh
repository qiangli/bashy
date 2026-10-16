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
PYTHONDONTWRITEBYTECODE=1 python3 "$here/_shared/go-service-exercise-test.py" || rc=1
# The go kind's toolchain + go.mod-directive gates, which otherwise only ever
# run on a release runner.
"$here/_shared/kinds/go/toolchain-gate-test.sh" || rc=1
for t in "${tools[@]}"; do
	if [[ $t == ollama ]]; then
		python3 "$here/ollama/test_recipe.py" || rc=1
		python3 "$here/ollama/test_native_gate.py" || rc=1
		PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s "$here/ollama" -p test_compiler_evidence.py -v || rc=1
		PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s "$here/ollama" -p test_audit.py -v || rc=1
	fi
	"$here/render.sh" "$t" "$tmp/$t" >/dev/null
	if ! diff -r "$tmp/$t" "$here/$t/fork"; then
		echo "check: $t: fork/ is stale — run builds/render.sh $t" >&2
		rc=1
		continue
	fi
	# Structural checks the regenerate-and-diff cannot make. A bug in
	# conditional rendering leaks the steps of a SUPPRESSED job into its
	# predecessor: the output stays deterministic (so the diff reproduces) and
	# stays valid YAML (so actionlint accepts it), but the workflow is wrong.
	# Both invariants below are fixed by the skeleton's shape: every job that
	# checks out `src` runs the pin gate exactly once, and the job set is
	# `pin build release` plus `audit` iff the tool has an audit fragment.
	wf=$tmp/$t/.github/workflows/bashy-build.yml
	jobs=$(awk '/^jobs:$/ {j = 1; next} j && /^  [a-z][a-z0-9_-]*:$/ {gsub(/[ :]/, ""); printf "%s ", $0}' "$wf")
	gates=$(grep -c 'name: verify HEAD == pinned commit' "$wf" || true)
	kind=$(
		set +u
		# shellcheck source=/dev/null
		. "$here/$t/tool.env" >/dev/null 2>&1
		echo "$KIND"
	)
	if [[ -f $here/$t/audit.yml || -f $here/_shared/kinds/$kind/audit.yml ]]; then
		want_jobs="pin build audit release " want_gates=2
	else
		want_jobs="pin build release " want_gates=1
	fi
	if [[ $jobs != "$want_jobs" ]]; then
		echo "check: $t: rendered jobs [$jobs] != expected [$want_jobs]" >&2
		rc=1
	fi
	if [[ $gates != "$want_gates" ]]; then
		echo "check: $t: $gates pin-gate steps in the workflow, expected $want_gates (a suppressed job's steps leaked?)" >&2
		rc=1
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
