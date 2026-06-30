#!/bin/bash
set -euo pipefail

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
task=
env_name=
tool=
out=results/agent-shell-pilot.jsonl

usage() {
  cat <<'USAGE'
usage: eval/agent-shell/run-task.sh --task NAME --env bashy-agentos|gnu-bash53 --tool TOOL [--out FILE]
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --task) task=$2; shift 2 ;;
    --env) env_name=$2; shift 2 ;;
    --tool) tool=$2; shift 2 ;;
    --out) out=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done

if [[ -z "$task" || -z "$env_name" || -z "$tool" ]]; then
  usage >&2
  exit 2
fi

task_dir="$repo/eval/agent-shell/tasks/$task"
if [[ ! -x "$task_dir/setup.sh" || ! -x "$task_dir/verify.sh" || ! -f "$task_dir/prompt.md" ]]; then
  printf 'invalid task: %s\n' "$task" >&2
  exit 2
fi

tool_bin=$(command -v "$tool" 2>/dev/null || true)
if [[ -z "$tool_bin" ]]; then
  printf 'tool not found on PATH before harness isolation: %s\n' "$tool" >&2
  exit 2
fi

case "$env_name" in
  bashy-agentos)
    eval_shell="$repo/bin/bashy"
    [[ -x "$eval_shell" ]] || { printf 'missing bashy: %s\n' "$eval_shell" >&2; exit 2; }
    ;;
  gnu-bash53)
    eval_shell="${GNU_BASH53:-}"
    [[ -n "$eval_shell" && -x "$eval_shell" ]] || { printf 'set GNU_BASH53 to a real GNU Bash 5.3 binary\n' >&2; exit 2; }
    version=$("$eval_shell" --version | sed -n '1p')
    [[ "$version" == *"GNU bash, version 5.3"* ]] || { printf 'GNU_BASH53 is not GNU Bash 5.3: %s\n' "$version" >&2; exit 2; }
    ;;
  *)
    printf 'unknown env: %s\n' "$env_name" >&2
    exit 2
    ;;
esac

run_root="${TMPDIR:-/tmp}/bashy-agent-shell-eval"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$task-$env_name-$tool-$$"
work="$run_root/$run_id/work"
logs="$run_root/$run_id/logs"
wrap="$run_root/$run_id/bin"
mkdir -p "$work" "$logs" "$wrap" "$(dirname "$out")"

"$task_dir/setup.sh" "$work"

cat >"$wrap/bash" <<'WRAP'
#!/bin/bash
set -euo pipefail
printf '%s\t%s\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$PWD" "$*" >>"${EVAL_COMMAND_LOG:?}"
exec "${EVAL_SHELL:?}" "$@"
WRAP
cat >"$wrap/sh" <<'WRAP'
#!/bin/bash
set -euo pipefail
printf '%s\t%s\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$PWD" "$*" >>"${EVAL_COMMAND_LOG:?}"
exec "${EVAL_SHELL:?}" "$@"
WRAP
chmod +x "$wrap/bash" "$wrap/sh"

prompt=$(sed \
  -e "s#__WORKSPACE__#$work#g" \
  -e "s#__EVAL_SHELL__#$eval_shell#g" \
  -e "s#__ENV_NAME__#$env_name#g" \
  "$task_dir/prompt.md")

export EVAL_ENV="$env_name"
export EVAL_SHELL="$eval_shell"
export EVAL_COMMAND_LOG="$logs/commands.tsv"
export DHNT_AGENT=1
export BASHY_ADVISOR=1
export PATH="$wrap:$repo/bin:/usr/bin:/bin"
export SHELL="$wrap/bash"

start_epoch=$(date +%s)
started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
tool_exit=0

case "$tool" in
  codex)
    "$tool_bin" exec --json --skip-git-repo-check --sandbox workspace-write -C "$work" "$prompt" >"$logs/tool.jsonl" 2>"$logs/tool.stderr" || tool_exit=$?
    ;;
  claude)
    "$tool_bin" -p --output-format stream-json --permission-mode bypassPermissions --add-dir "$work" "$prompt" >"$logs/tool.jsonl" 2>"$logs/tool.stderr" || tool_exit=$?
    ;;
  agy)
    "$tool_bin" --print --dangerously-skip-permissions --print-timeout 20m --add-dir "$work" "$prompt" >"$logs/tool.out" 2>"$logs/tool.stderr" || tool_exit=$?
    ;;
  opencode)
    "$tool_bin" run --format json --dangerously-skip-permissions --dir "$work" "$prompt" >"$logs/tool.jsonl" 2>"$logs/tool.stderr" || tool_exit=$?
    ;;
  aider)
    (cd "$work" && "$tool_bin" --yes-always --no-check-update --no-auto-commits --message "$prompt" >"$logs/tool.out" 2>"$logs/tool.stderr") || tool_exit=$?
    ;;
  *)
    printf 'unknown tool: %s\n' "$tool" >&2
    exit 2
    ;;
esac

verifier_exit=0
/bin/bash "$task_dir/verify.sh" "$work" >"$logs/verify.out" 2>"$logs/verify.stderr" || verifier_exit=$?
finished_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
end_epoch=$(date +%s)
wall_time_sec=$((end_epoch - start_epoch))
command_count=0
if [[ -f "$logs/commands.tsv" ]]; then
  command_count=$(wc -l <"$logs/commands.tsv" | tr -d ' ')
fi
success=false
if [[ "$tool_exit" -eq 0 && "$verifier_exit" -eq 0 ]]; then
  success=true
fi

failure_mode=none
if [[ "$tool_exit" -ne 0 ]]; then
  failure_mode=tool-exit
elif [[ "$verifier_exit" -ne 0 ]]; then
  failure_mode=verifier-fail
fi

hash_cmd=shasum
command -v sha256sum >/dev/null 2>&1 && hash_cmd=sha256sum
verifier_hash=$($hash_cmd "$logs/verify.out" | awk '{print $1}')

printf '{"run_id":"%s","task_id":"%s","tool":"%s","env":"%s","started_at":"%s","finished_at":"%s","wall_time_sec":%s,"success":%s,"tool_exit":%s,"verifier_exit":%s,"failure_mode":"%s","command_count":%s,"shell_path_observed":"%s","workspace":"%s","log_dir":"%s","verifier_output_hash":"%s"}\n' \
  "$run_id" "$task" "$tool" "$env_name" "$started_at" "$finished_at" "$wall_time_sec" "$success" "$tool_exit" "$verifier_exit" "$failure_mode" "$command_count" "$eval_shell" "$work" "$logs" "$verifier_hash" >>"$out"

printf 'run_id=%s success=%s tool_exit=%s verifier_exit=%s logs=%s\n' "$run_id" "$success" "$tool_exit" "$verifier_exit" "$logs"

if [[ "$success" == true ]]; then
  exit 0
fi
exit 1
