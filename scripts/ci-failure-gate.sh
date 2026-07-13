#!/usr/bin/env bashy
set -euo pipefail

failed_run_id="${1:?failed run id required}"
branch="${2:-}"
timeout="${DHNT_CI_GATE_TIMEOUT:-25m}"
poll="${DHNT_CI_GATE_POLL:-20s}"

die() {
	echo "ci-failure-gate: $*" >&2
	exit 1
}

preflight() {
	command -v gh >/dev/null 2>&1 || die "missing required command 'gh'. Install GitHub CLI on the repair host."
	command -v jq >/dev/null 2>&1 || die "missing required command 'jq'. Install jq on the repair host."
	[[ "$failed_run_id" =~ ^[0-9]+$ ]] || die "failed run id must be numeric, got '$failed_run_id'."
	[[ -n "${GH_TOKEN:-${GITHUB_TOKEN:-}}" ]] || gh auth status >/dev/null 2>&1 ||
		die "no GH_TOKEN/GITHUB_TOKEN is set and gh is not authenticated. Configure a token that can read source-repo Actions runs."
}

if [[ -z "$branch" || "$branch" == "null" ]]; then
	branch="$(git branch --show-current 2>/dev/null || true)"
fi
[[ -n "$branch" ]] || die "branch is empty and could not be inferred from git."

preflight

duration_seconds() {
	local raw="$1" n unit
	n="${raw%[smh]}"
	unit="${raw#$n}"
	if [[ -z "$n" || "$n" == "$raw" ]]; then
		printf '%s\n' "$raw"
		return
	fi
	case "$unit" in
		s) printf '%s\n' "$n" ;;
		m) printf '%s\n' "$((n * 60))" ;;
		h) printf '%s\n' "$((n * 3600))" ;;
		*) printf '%s\n' "$raw" ;;
	esac
}

start_seconds="$SECONDS"
timeout_seconds="$(duration_seconds "$timeout")"

latest_newer_run() {
	gh run list --branch "$branch" --limit 20 \
		--json databaseId,status,conclusion,workflowName,headSha,url,createdAt |
		jq -r --argjson failed "$failed_run_id" '
			[.[] | select(.databaseId > $failed)] |
			sort_by(.databaseId) |
			reverse |
			.[0] // empty
		'
}

gh run view "$failed_run_id" --json databaseId,url >/dev/null ||
	die "GitHub token cannot read failed run $failed_run_id. Grant Actions:Read on the source repo before waiting for repair CI."

while :; do
	if (( SECONDS - start_seconds > timeout_seconds )); then
		echo "ci-failure-gate: timed out waiting for a newer CI run on branch $branch after failed run $failed_run_id" >&2
		exit 124
	fi

	run="$(latest_newer_run)"
	if [[ -z "$run" || "$run" == "null" ]]; then
		echo "ci-failure-gate: no newer CI run yet on branch $branch after failed run $failed_run_id" >&2
		sleep "$poll"
		continue
	fi

	run_id="$(jq -r '.databaseId' <<<"$run")"
	status="$(jq -r '.status' <<<"$run")"
	conclusion="$(jq -r '.conclusion // ""' <<<"$run")"
	url="$(jq -r '.url' <<<"$run")"

	echo "ci-failure-gate: checking newer run $run_id ($status/$conclusion) $url"
	if [[ "$status" == "completed" ]]; then
		if [[ "$conclusion" == "success" ]]; then
			exit 0
		fi
		exit 1
	fi

	gh run watch "$run_id" --exit-status
	exit $?
done
