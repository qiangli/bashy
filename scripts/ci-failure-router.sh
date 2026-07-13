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
handoff_root="${DHNT_CI_HANDOFF_DIR:-$HOME/.bashy/ci-repair}"
lockdir="${DHNT_CI_ROUTER_LOCKDIR:-/tmp/dhnt-ci-failure-router.lock}"
prompt_template="${DHNT_CI_CONDUCTOR_PROMPT:-$script_dir/ci-failure-conductor-brief.md}"

usage() {
	cat <<'EOF'
usage: ci-failure-router.sh [--once] [--dry-run] [--issue NUMBER]

Cheap deterministic CI failure router. Claims collector issues, chooses the
on-shift premium conductor, writes a handoff brief, and launches the conductor.
EOF
}

die() {
	echo "ci-failure-router: $*" >&2
	exit 1
}

dry_run=0
target_issue=""
while (($#)); do
	case "$1" in
		--once) shift ;;
		--dry-run) dry_run=1; shift ;;
		--issue)
			[[ $# -ge 2 ]] || die "--issue requires a collector issue number"
			target_issue="$2"
			shift 2
			;;
		-h|--help) usage; exit 0 ;;
		*) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
	esac
done

require_command() {
	local cmd="$1" hint="$2"
	command -v "$cmd" >/dev/null 2>&1 || die "missing required command '$cmd'. $hint"
}

preflight() {
	local auth_output
	local shift_hours

	require_command gh "Install GitHub CLI on the repair host."
	require_command jq "Install jq on the repair host."
	require_command bashy "Install bashy on the repair host and ensure it is on PATH."

	[[ -f "$prompt_template" ]] || die "missing conductor prompt template: $prompt_template"
	[[ -d "$dhnt_root" ]] || die "DHNT_CI_ROOT does not exist: $dhnt_root"
	shift_hours="${DHNT_CI_SHIFT_HOURS:-1}"
	[[ "$shift_hours" =~ ^[0-9]+$ && "$shift_hours" -gt 0 ]] ||
		die "DHNT_CI_SHIFT_HOURS must be a positive integer, got '$shift_hours'."

	if [[ -z "${GH_TOKEN:-${GITHUB_TOKEN:-}}" ]]; then
		auth_output="$(gh auth status 2>&1)" || die "no GH_TOKEN/GITHUB_TOKEN is set and gh is not authenticated. Configure a token with Metadata:Read and Issues:Read+Write on $collector."
	else
		auth_output="$(gh auth status 2>&1 || true)"
	fi

	gh api "repos/${collector}" >/dev/null ||
		die "GitHub token cannot read $collector metadata. Grant Metadata:Read on the collector repo. gh auth status: $auth_output"

	gh label list -R "$collector" --limit 1 >/dev/null ||
		die "GitHub token cannot read $collector labels. Grant Issues:Read+Write; label access is required before repair claims."

	gh issue list -R "$collector" --state open --limit 1 >/dev/null ||
		die "GitHub token cannot read $collector issues. Grant Issues:Read+Write on the collector repo."

	if [[ -n "$target_issue" ]]; then
		gh issue view "$target_issue" -R "$collector" --json number,state,labels >/dev/null ||
			die "GitHub token cannot read target collector issue #$target_issue in $collector. Check the repository_dispatch payload and Issues:Read permission."
	fi

	if ! bashy commands chat >/dev/null 2>&1; then
		die "bashy does not expose the 'chat' conductor launcher on this host; router cannot start repair sessions."
	fi
}

preflight

if ! mkdir "$lockdir" 2>/dev/null; then
	echo "ci-failure-router: another router is already running ($lockdir)"
	exit 0
fi
trap 'rmdir "$lockdir" 2>/dev/null || true' EXIT

now_epoch() { date +%s; }
now_iso() { date -u +"%Y-%m-%dT%H:%M:%SZ"; }

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

ensure_labels() {
	local label color desc
	while IFS='|' read -r label color desc; do
		gh label create "$label" -R "$collector" --color "$color" --description "$desc" >/dev/null 2>&1 ||
			gh label edit "$label" -R "$collector" --color "$color" --description "$desc" >/dev/null ||
			die "GitHub token cannot create/edit label '$label' in $collector. Grant Issues:Read+Write on the collector repo."
	done <<'EOF'
repair-running|FBCA04|Automated repair attempt is running
repair-done|0E8A16|Automated repair converged and closed the issue
repair-failed|D73A4A|Automated repair was attempted but did not converge
repair-paused|5319E7|Do not auto-repair this issue
EOF
}

has_label() {
	local labels_csv="$1" label="$2"
	[[ ",$labels_csv," == *",$label,"* ]]
}

state_dir_for() {
	local issue="$1" safe
	safe="${collector//\//__}"
	printf '%s/%s/%s\n' "$handoff_root" "$safe" "$issue"
}

lease_file_for() {
	printf '%s/lease.json\n' "$(state_dir_for "$1")"
}

active_roster() {
	local shift_hours roster_csv shift_index count idx conductor workers
	shift_hours="${DHNT_CI_SHIFT_HOURS:-1}"
	roster_csv="${DHNT_CI_SHIFT_ROSTER:-codex,claude}"
	IFS=',' read -ra roster <<<"$roster_csv"
	count="${#roster[@]}"
	if (( count == 0 )); then
		roster=(codex claude)
		count=2
	fi
	shift_index="$(( $(now_epoch) / (shift_hours * 3600) ))"
	idx="$(( shift_index % count ))"
	conductor="${roster[$idx]}"
	conductor="${conductor//[[:space:]]/}"
	case "$conductor" in
		codex) workers="${DHNT_CI_EVEN_WORKERS:-claude,agy,opencode,aider}" ;;
		claude) workers="${DHNT_CI_ODD_WORKERS:-codex,agy,opencode,aider}" ;;
		*) workers="${DHNT_CI_WORKERS:-codex,claude,agy,opencode,aider}" ;;
	esac
	printf '%s|%s|%s|%s\n' "$conductor" "$workers" "$shift_index" "$shift_hours"
}

repo_path_for() {
	local source_repo="$1" pair repo rel
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

latest_failure_text() {
	local issue="$1"
	gh issue view "$issue" -R "$collector" --json body,comments --jq '
		([.body] + [.comments[].body]) |
		reverse[] |
		select(test("- Source repo:")) |
		.
	' | head -n1
}

field_from_text() {
	local name="$1" text="$2"
	printf '%s\n' "$text" |
		sed -nE "s/^[[:space:]]*-[[:space:]]*${name}:[[:space:]]*//p" |
		head -n1
}

lease_is_stale() {
	local issue="$1" lease started stale_after shift_seconds age
	lease="$(lease_file_for "$issue")"
	if [[ ! -s "$lease" ]]; then
		return 0
	fi
	started="$(jq -r '.started_epoch // 0' "$lease" 2>/dev/null || printf 0)"
	if [[ "$started" == "0" ]]; then
		return 0
	fi
	shift_seconds="$(( ${DHNT_CI_SHIFT_HOURS:-1} * 3600 ))"
	stale_after="$(duration_seconds "${DHNT_CI_STALE_AFTER:-$((shift_seconds + 900))s}")"
	age="$(( $(now_epoch) - started ))"
	(( age > stale_after ))
}

write_lease() {
	local issue="$1" conductor="$2" workers="$3" shift_index="$4" shift_hours="$5" source_repo="$6" run_id="$7" state_dir lease shift_end
	state_dir="$(state_dir_for "$issue")"
	lease="$(lease_file_for "$issue")"
	mkdir -p "$state_dir"
	shift_end="$(( (shift_index + 1) * shift_hours * 3600 ))"
	jq -n \
		--arg conductor "$conductor" \
		--arg workers "$workers" \
		--arg collector "$collector" \
		--arg issue "$issue" \
		--arg source_repo "$source_repo" \
		--arg run_id "$run_id" \
		--arg started_at "$(now_iso)" \
		--argjson started_epoch "$(now_epoch)" \
		--argjson shift_index "$shift_index" \
		--argjson shift_ends_epoch "$shift_end" \
		'{conductor:$conductor,workers:$workers,collector:$collector,issue:($issue|tonumber),source_repo:$source_repo,run_id:$run_id,started_at:$started_at,started_epoch:$started_epoch,shift_index:$shift_index,shift_ends_epoch:$shift_ends_epoch}' \
		>"$lease"
}

write_brief() {
	local issue="$1" issue_url="$2" source_repo="$3" workflow="$4" branch="$5" sha="$6" run_url="$7" run_id="$8" repo_path="$9" conductor="${10}" workers="${11}" stale="${12}" state_dir brief
	state_dir="$(state_dir_for "$issue")"
	brief="$state_dir/brief.md"
	mkdir -p "$state_dir"
	cat >"$brief" <<EOF
# CI Repair Handoff Brief

- Collector repo: $collector
- Collector issue: $issue
- Collector issue URL: $issue_url
- Source repo: $source_repo
- Local repo path: $repo_path
- Workflow: $workflow
- Branch: $branch
- SHA: $sha
- Failed run: $run_url
- Failed run id: $run_id
- On-shift conductor: $conductor
- Worker pool: $workers
- Repair mode: ${DHNT_CI_REPAIR_MODE:-weave}
- Handoff directory: $state_dir
- Stale recovery claim: $stale
- Created at: $(now_iso)

## Required Acceptance

Repair the failed workflow without clobbering the live checkout. Use isolated
\`bashy weave\` workspaces for workers. Push the repair only after verified merge,
then wait for a newer GitHub Actions run on branch \`$branch\` to pass.

Gate helper:

\`\`\`sh
$script_dir/ci-failure-gate.sh $run_id '$branch'
\`\`\`

## Collector Updates

Write recovery summaries, progress notes, blockers, and shift handoff notes to:

\`\`\`sh
gh issue comment $issue -R $collector --body '...'
\`\`\`

Use labels:

- \`repair-done\` and close when converged.
- \`repair-failed\` with blocker summary when not converged.
- \`repair-paused\` when unsafe to continue.
EOF
	printf '%s\n' "$brief"
}

claimable_issues() {
	if [[ -n "$target_issue" ]]; then
		gh issue view "$target_issue" -R "$collector" \
			--json number,url,labels,state \
			--jq 'select(.state == "OPEN") | [.number, .url, ([.labels[].name] | join(","))] | @tsv'
	else
		gh issue list -R "$collector" --state open --label ci-failure --limit 100 \
			--json number,url,labels \
			--jq '.[] | [.number, .url, ([.labels[].name] | join(","))] | @tsv'
	fi
}

run_issue() {
	local issue="$1" issue_url="$2" labels="$3" text source_repo workflow branch sha run_url run_id repo_path roster conductor workers shift_index shift_hours state_dir brief stale code instruction
	RUN_ISSUE_CLAIMED=0

	if ! has_label "$labels" ci-failure; then
		return 0
	fi
	if has_label "$labels" repair-done || has_label "$labels" repair-paused || has_label "$labels" repair-failed; then
		return 0
	fi
	stale=0
	if has_label "$labels" repair-running; then
		if lease_is_stale "$issue"; then
			stale=1
		else
			return 0
		fi
	fi

	text="$(latest_failure_text "$issue")"
	source_repo="$(field_from_text "Source repo" "$text")"
	workflow="$(field_from_text "Workflow" "$text")"
	branch="$(field_from_text "Branch" "$text")"
	sha="$(field_from_text "SHA" "$text")"
	run_url="$(field_from_text "Run" "$text")"
	run_id="$(field_from_text "Run id" "$text")"

	if [[ -z "$source_repo" || -z "$run_id" ]]; then
		gh issue comment "$issue" -R "$collector" --body "Auto-repair router skipped: could not parse source repo/run id from the latest failure report."
		gh issue edit "$issue" -R "$collector" --add-label repair-paused >/dev/null
		return 0
	fi

	repo_path="$(repo_path_for "$source_repo")"
	if [[ ! -d "$repo_path/.git" ]]; then
		gh issue comment "$issue" -R "$collector" --body "Auto-repair router skipped: local checkout not found at \`$repo_path\` for \`$source_repo\`."
		gh issue edit "$issue" -R "$collector" --add-label repair-paused >/dev/null
		return 0
	fi

	roster="$(active_roster)"
	IFS='|' read -r conductor workers shift_index shift_hours <<<"$roster"

	if (( dry_run )); then
		printf 'issue=%s repo=%s conductor=%s workers=%s stale=%s\n' "$issue" "$repo_path" "$conductor" "$workers" "$stale"
		RUN_ISSUE_CLAIMED=1
		return 0
	fi

	write_lease "$issue" "$conductor" "$workers" "$shift_index" "$shift_hours" "$source_repo" "$run_id"
	brief="$(write_brief "$issue" "$issue_url" "$source_repo" "$workflow" "$branch" "$sha" "$run_url" "$run_id" "$repo_path" "$conductor" "$workers" "$stale")"
	RUN_ISSUE_CLAIMED=1

	gh issue edit "$issue" -R "$collector" --add-label repair-running >/dev/null
	gh issue edit "$issue" -R "$collector" --remove-label repair-failed >/dev/null 2>&1 || true
	if (( stale )); then
		gh issue comment "$issue" -R "$collector" --body "Auto-repair router recovered a stale running claim on $(hostname). New conductor: \`$conductor\`. Brief: \`$brief\`."
	else
		gh issue comment "$issue" -R "$collector" --body "Auto-repair router claimed this on $(hostname). On-shift conductor: \`$conductor\`. Workers available: \`$workers\`. Brief: \`$brief\`."
	fi

	instruction="$(cat "$prompt_template")"
	set +e
	bashy chat \
		--agent "$conductor" \
		--cwd "$repo_path" \
		--sandbox "${DHNT_CI_SANDBOX:-danger-full-access}" \
		--timeout "${DHNT_CI_CONDUCTOR_TIMEOUT:-2h}" \
		--file "$brief" \
		--instruction "$instruction"
	code=$?
	set -e

	if (( code == 0 )); then
		gh issue comment "$issue" -R "$collector" --body "Conductor session \`$conductor\` exited cleanly. The conductor is responsible for final labels/closure; router leaves \`repair-running\` unless the issue was closed or relabeled by the conductor."
	else
		gh issue comment "$issue" -R "$collector" --body "Conductor session \`$conductor\` exited non-zero ($code). Incoming conductor must run recovery gate before continuing."
	fi
}

ensure_labels

processed=0
while IFS=$'\t' read -r issue issue_url labels; do
	[[ -z "$issue" ]] && continue
	RUN_ISSUE_CLAIMED=0
	run_issue "$issue" "$issue_url" "$labels"
	if (( RUN_ISSUE_CLAIMED )); then
		processed="$((processed + 1))"
	fi
	if (( processed >= limit )); then
		break
	fi
done < <(claimable_issues)

if (( processed == 0 )); then
	echo "ci-failure-router: no claimable CI failure issues"
fi
