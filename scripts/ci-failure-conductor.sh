#!/usr/bin/env bashy
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "$0")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"
config="${DHNT_CI_CONDUCTOR_CONFIG:-$repo_root/config/ci-failure-conductor.env}"

if [[ -f "$config" ]]; then
	# shellcheck disable=SC1090
	source "$config"
fi

collector="${DHNT_CI_COLLECTOR_REPO:-qiangli/dhnt-ci-failures}"
dhnt_root="${DHNT_CI_ROOT:-/Users/qiangli/projects/poc/dhnt}"
limit="${DHNT_CI_LIMIT:-1}"
lockdir="${DHNT_CI_LOCKDIR:-/tmp/dhnt-ci-failure-conductor.lock}"

usage() {
	cat <<'EOF'
usage: ci-failure-conductor.sh [--once] [--dry-run]

Poll the DHNT CI failure collector, claim unclaimed failure issues, choose the
active conductor by local hour parity, and launch bashy supervise against the
source repo.
EOF
}

dry_run=0
while (($#)); do
	case "$1" in
		--once) shift ;;
		--dry-run) dry_run=1; shift ;;
		-h|--help) usage; exit 0 ;;
		*) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
	esac
done

if ! mkdir "$lockdir" 2>/dev/null; then
	echo "ci-failure-conductor: another dispatcher is already running ($lockdir)"
	exit 0
fi
trap 'rmdir "$lockdir" 2>/dev/null || true' EXIT

ensure_labels() {
	local label color desc
	while IFS='|' read -r label color desc; do
		gh label create "$label" -R "$collector" --color "$color" --description "$desc" >/dev/null 2>&1 ||
			gh label edit "$label" -R "$collector" --color "$color" --description "$desc" >/dev/null
	done <<'EOF'
repair-running|FBCA04|Automated repair attempt is running
repair-done|0E8A16|Automated repair converged and closed the issue
repair-failed|D73A4A|Automated repair was attempted but did not converge
repair-paused|5319E7|Do not auto-repair this issue
EOF
}

active_roster() {
	local hour conductor workers
	hour="$(date +%H)"
	if (( 10#$hour % 2 == 0 )); then
		conductor="${DHNT_CI_EVEN_CONDUCTOR:-codex}"
		workers="${DHNT_CI_EVEN_WORKERS:-claude,agy,opencode,aider}"
	else
		conductor="${DHNT_CI_ODD_CONDUCTOR:-claude}"
		workers="${DHNT_CI_ODD_WORKERS:-codex,agy,opencode,aider}"
	fi
	printf '%s|%s\n' "$conductor" "$workers"
}

repo_path_for() {
	local source_repo="$1" mapping pair repo rel
	IFS=',' read -ra pairs <<<"${DHNT_CI_REPOS:-}"
	for pair in "${pairs[@]}"; do
		repo="${pair%%=*}"
		rel="${pair#*=}"
		if [[ "$repo" == "$source_repo" ]]; then
			printf '%s/%s\n' "$dhnt_root" "$rel"
			return 0
		fi
	done
	printf '%s/%s\n' "$dhnt_root" "${source_repo##*/}"
}

latest_issue_text() {
	local issue="$1"
	gh issue view "$issue" -R "$collector" --json body,comments --jq '
		if (.comments | length) > 0 then .comments[-1].body else .body end
	'
}

field_from_text() {
	local name="$1" text="$2"
	printf '%s\n' "$text" |
		sed -nE "s/^[[:space:]]*-[[:space:]]*${name}:[[:space:]]*//p" |
		head -n1
}

sq() {
	printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

run_issue() {
	local issue="$1" issue_url="$2" text source_repo workflow branch sha run_url run_id repo_path roster conductor workers gate task goal

	text="$(latest_issue_text "$issue")"
	source_repo="$(field_from_text "Source repo" "$text")"
	workflow="$(field_from_text "Workflow" "$text")"
	branch="$(field_from_text "Branch" "$text")"
	sha="$(field_from_text "SHA" "$text")"
	run_url="$(field_from_text "Run" "$text")"
	run_id="$(field_from_text "Run id" "$text")"

	if [[ -z "$source_repo" || -z "$run_id" ]]; then
		gh issue comment "$issue" -R "$collector" --body "Auto-repair skipped: could not parse source repo/run id from the latest failure report."
		gh issue edit "$issue" -R "$collector" --add-label repair-paused >/dev/null
		return 0
	fi

	repo_path="$(repo_path_for "$source_repo")"
	if [[ ! -d "$repo_path/.git" ]]; then
		gh issue comment "$issue" -R "$collector" --body "Auto-repair skipped: local checkout not found at \`$repo_path\` for \`$source_repo\`."
		gh issue edit "$issue" -R "$collector" --add-label repair-paused >/dev/null
		return 0
	fi

	roster="$(active_roster)"
	conductor="${roster%%|*}"
	workers="${roster#*|}"

	gh issue edit "$issue" -R "$collector" --add-label repair-running --remove-label repair-failed >/dev/null
	gh issue comment "$issue" -R "$collector" --body "Auto-repair claimed on $(hostname) by conductor \`$conductor\` with workers \`$workers\`."

	gate="$(sq "$script_dir/ci-failure-gate.sh") $(sq "$run_id") $(sq "$branch")"
	goal="Fix the CI failure reported in $issue_url for $source_repo workflow $workflow. Failed run: $run_url. Branch: $branch. SHA: $sha. Use bashy for commands. Commit and push the fix if code changes are required."
	task="fix-ci| Diagnose and fix the failing CI run, push the repair, and wait for the replacement run to pass :: $gate"

	if (( dry_run )); then
		printf 'issue=%s repo=%s conductor=%s workers=%s gate=%s\n' "$issue" "$repo_path" "$conductor" "$workers" "$gate"
		return 0
	fi

	set +e
	(
		cd "$repo_path"
		IFS=',' read -ra worker_list <<<"$workers"
		cmd=(bashy supervise
			--goal "$goal"
			--supervisor "$conductor"
			--sandbox "${DHNT_CI_SANDBOX:-danger-full-access}"
			--turn-timeout "${DHNT_CI_TURN_TIMEOUT:-45m}"
			--max-attempts "${DHNT_CI_MAX_ATTEMPTS:-2}"
			--out docs
			--task "$task")
		for worker in "${worker_list[@]}"; do
			worker="${worker//[[:space:]]/}"
			[[ -n "$worker" ]] && cmd+=(--worker "$worker")
		done
		BASHY_SUPERVISE_GATE_TIMEOUT="${DHNT_CI_GATE_TIMEOUT:-25m}" "${cmd[@]}"
	)
	local code=$?
	set -e
	if (( code == 0 )); then
		gh issue comment "$issue" -R "$collector" --body "Auto-repair converged and the replacement CI gate passed."
		gh issue edit "$issue" -R "$collector" --remove-label repair-running --remove-label repair-failed --add-label repair-done >/dev/null
		gh issue close "$issue" -R "$collector" --comment "Closed by the CI auto-repair dispatcher." >/dev/null
	else
		gh issue comment "$issue" -R "$collector" --body "Auto-repair did not converge. Exit code: $code. Leaving issue open for handoff."
		gh issue edit "$issue" -R "$collector" --remove-label repair-running --add-label repair-failed >/dev/null
	fi
}

ensure_labels

issues="$(
	gh issue list -R "$collector" --state open --label ci-failure --limit 100 \
		--json number,url,labels \
		--jq '
			.[] |
			select(([.labels[].name] | index("repair-running") | not)) |
			select(([.labels[].name] | index("repair-done") | not)) |
			select(([.labels[].name] | index("repair-paused") | not)) |
			[.number, .url] | @tsv
		' |
		head -n "$limit"
)"

if [[ -z "$issues" ]]; then
	echo "ci-failure-conductor: no claimable CI failure issues"
	exit 0
fi

while IFS=$'\t' read -r issue issue_url; do
	[[ -n "$issue" ]] && run_issue "$issue" "$issue_url"
done <<<"$issues"
