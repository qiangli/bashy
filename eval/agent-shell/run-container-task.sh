#!/bin/bash
set -euo pipefail

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
task=
env_name=
tool=
out=results/agent-shell-container.jsonl
run_base="${HOME}/tests/bashy-eval/runs"
max_retries=1

usage() {
  cat <<'USAGE'
usage: eval/agent-shell/run-container-task.sh --task NAME --env bashy-current|bashy-v0.4.0|gnu-bash53 --tool codex|claude|agy [--out FILE] [--run-base DIR]
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --task) task=$2; shift 2 ;;
    --env) env_name=$2; shift 2 ;;
    --tool) tool=$2; shift 2 ;;
    --out) out=$2; shift 2 ;;
    --run-base) run_base=$2; shift 2 ;;
    --max-retries) max_retries=$2; shift 2 ;;
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

case "$env_name" in
  bashy-current) image=bashy-agent-shell:bashy-current ;;
  bashy-v0.4.0) image=bashy-agent-shell:bashy-v0.4.0 ;;
  gnu-bash53) image=bashy-agent-shell:gnu-bash53 ;;
  *) printf 'unknown env: %s\n' "$env_name" >&2; exit 2 ;;
esac

tool_bin=$(command -v "$tool" 2>/dev/null || true)
if [[ -z "$tool_bin" ]]; then
  printf 'tool not found: %s\n' "$tool" >&2
  exit 2
fi

if [[ ! -x "$repo/bin/bashy" ]]; then
  printf 'missing host bashy with podman support: %s\n' "$repo/bin/bashy" >&2
  exit 2
fi

mkdir -p "$run_base" "$(dirname "$out")"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$task-$env_name-$tool-$$"
run_root="$run_base/$run_id"
work="$run_root/work"
logs="$run_root/logs"
bin_dir="$run_root/bin"
mkdir -p "$work" "$logs" "$bin_dir"

"$task_dir/setup.sh" "$work"

cat >"$bin_dir/eval-shell" <<'WRAP'
#!/bin/bash
set -euo pipefail

host_pwd=$PWD
case "$host_pwd" in
  "$EVAL_WORK") container_pwd=/workspace ;;
  "$EVAL_WORK"/*) container_pwd="/workspace${host_pwd#"$EVAL_WORK"}" ;;
  *) container_pwd=/workspace ;;
esac

printf '%s\t%s\t%s\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$host_pwd" "$container_pwd" "$*" >>"${EVAL_COMMAND_LOG:?}"
exec "${BASHY_HOST:?}" podman run --rm \
  -v "${EVAL_WORK:?}:/workspace:rw" \
  -w "$container_pwd" \
  "${EVAL_IMAGE:?}" "$@"
WRAP
chmod +x "$bin_dir/eval-shell"

cat >"$bin_dir/bash" <<'WRAP'
#!/bin/bash
exec "$(dirname "$0")/eval-shell" "$@"
WRAP
cat >"$bin_dir/sh" <<'WRAP'
#!/bin/bash
exec "$(dirname "$0")/eval-shell" "$@"
WRAP
chmod +x "$bin_dir/bash" "$bin_dir/sh"

initial_cwd=$work
if [[ -d "$work/nested/deep" ]]; then
  initial_cwd="$work/nested/deep"
fi

prompt_body=$(sed \
  -e "s#__WORKSPACE__#$work#g" \
  -e "s#__EVAL_SHELL__#$bin_dir/eval-shell#g" \
  -e "s#__ENV_NAME__#$env_name#g" \
  "$task_dir/prompt.md")

prompt=$(cat <<PROMPT
You are participating in a shell benchmark comparing current bashy against GNU Bash 5.3.

Critical execution rule:
- Run task shell commands only through this wrapper: $bin_dir/eval-shell
- The wrapper executes inside the selected container image: $image
- You may edit files in the workspace directly if needed, but every shell command used to inspect, run, test, or verify the task must go through the wrapper.
- Do not use the host /bin/bash, /bin/sh, zsh, Python, Ruby, Node, or other host interpreters for task work.
- If a command fails due to an API/rate-limit/tooling issue, report it plainly and retry only after a short wait.

$prompt_body
PROMPT
)

export BASHY_HOST="$repo/bin/bashy"
export EVAL_WORK="$work"
export EVAL_IMAGE="$image"
export EVAL_COMMAND_LOG="$logs/container-shell.tsv"
export PATH="$bin_dir:/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin"
export SHELL="$bin_dir/bash"

tool_exit=0
retry_count=0
retry_sleep_sec=0
rate_limit_count=0
attempt=0
start_epoch=$(date +%s)
started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)

while :; do
  attempt=$((attempt + 1))
  attempt_log="$logs/tool-attempt-$attempt"
  tool_exit=0
  case "$tool" in
    codex)
      "$tool_bin" exec --json --skip-git-repo-check --sandbox danger-full-access -C "$initial_cwd" "$prompt" >"$attempt_log.jsonl" 2>"$attempt_log.stderr" || tool_exit=$?
      ;;
    claude)
      (cd "$initial_cwd" && printf '%s\n' "$prompt" | "$tool_bin" -p --verbose --output-format stream-json --permission-mode bypassPermissions --add-dir "$work" >"$attempt_log.jsonl" 2>"$attempt_log.stderr") || tool_exit=$?
      ;;
    agy)
      (cd "$initial_cwd" && printf '%s\n' "$prompt" | "$tool_bin" --print --dangerously-skip-permissions --print-timeout 20m --add-dir "$work" >"$attempt_log.out" 2>"$attempt_log.stderr") || tool_exit=$?
      ;;
    *)
      printf 'unsupported approved pilot tool: %s\n' "$tool" >&2
      exit 2
      ;;
  esac

  cat "$attempt_log".* >"$logs/tool-combined-attempt-$attempt.log" 2>/dev/null || true
  rate_limit_hits=$(grep -Eic 'rate[ _-]?limit.*(denied|blocked|error|exceeded)|"api_error_status":"?[45][0-9][0-9]|429|overloaded|quota exceeded|api error:|timeout waiting' "$logs/tool-combined-attempt-$attempt.log" 2>/dev/null || true)
  rate_limit_count=$((rate_limit_count + rate_limit_hits))
  if [[ "$tool_exit" -eq 0 || "$attempt" -gt "$max_retries" || "$rate_limit_hits" -eq 0 ]]; then
    break
  fi
  retry_count=$((retry_count + 1))
  sleep_for=$((30 * retry_count))
  retry_sleep_sec=$((retry_sleep_sec + sleep_for))
  printf 'retry_after_rate_limit attempt=%s sleep_sec=%s\n' "$attempt" "$sleep_for" >>"$logs/retries.log"
  sleep "$sleep_for"
  tool_exit=0
done

cp "$task_dir/verify.sh" "$work/.verify.sh"
chmod +x "$work/.verify.sh"
verifier_exit=0
"$bin_dir/eval-shell" /workspace/.verify.sh /workspace >"$logs/verify.out" 2>"$logs/verify.stderr" || verifier_exit=$?

finished_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
end_epoch=$(date +%s)
wall_time_sec=$((end_epoch - start_epoch))

command_count=0
if [[ -f "$logs/container-shell.tsv" ]]; then
  command_count=$(wc -l <"$logs/container-shell.tsv" | tr -d ' ')
fi

tool_call_count=0
case "$tool" in
  codex)
    tool_call_count=$(grep -h '"command_execution"' "$logs"/tool-attempt-*.jsonl 2>/dev/null | wc -l | tr -d ' ' || true)
    ;;
  claude)
    tool_call_count=$(grep -hE '"tool_use"|"tool_result"' "$logs"/tool-attempt-*.jsonl 2>/dev/null | wc -l | tr -d ' ' || true)
    ;;
  agy)
    tool_call_count=$command_count
    ;;
esac

token_usage="not_parsed"
case "$tool" in
  codex)
    token_usage=$(python3 - "$logs" <<'PY' || true
import glob, json, sys
logs = sys.argv[1]
usage = None
for path in sorted(glob.glob(f"{logs}/tool-attempt-*.jsonl")):
    for line in open(path, errors="replace"):
        if '"turn.completed"' not in line:
            continue
        try:
            obj = json.loads(line)
        except Exception:
            continue
        usage = obj.get("usage") or usage
summary = json.dumps(usage if usage is not None else "not_parsed", separators=(",", ":"))
print(json.dumps(summary)[1:-1])
PY
)
    ;;
  claude)
    token_usage=$(python3 - "$logs" <<'PY' || true
import glob, json, sys
logs = sys.argv[1]
result = None
for path in sorted(glob.glob(f"{logs}/tool-attempt-*.jsonl")):
    for line in open(path, errors="replace"):
        if '"type":"result"' not in line:
            continue
        try:
            obj = json.loads(line)
        except Exception:
            continue
        if obj.get("type") == "result":
            result = obj
if result is None:
    summary = "not_parsed"
else:
    usage = result.get("usage") or {}
    summary = {
        "total_cost_usd": result.get("total_cost_usd"),
        "input_tokens": usage.get("input_tokens"),
        "cache_creation_input_tokens": usage.get("cache_creation_input_tokens"),
        "cache_read_input_tokens": usage.get("cache_read_input_tokens"),
        "output_tokens": usage.get("output_tokens"),
        "model_usage": result.get("modelUsage"),
    }
summary = json.dumps(summary, separators=(",", ":"))
print(json.dumps(summary)[1:-1])
PY
)
    ;;
  agy)
    token_usage=$(python3 - "$logs" <<'PY' || true
import glob, json, math, os, sys
logs = sys.argv[1]
chars = 0
for pattern in ("tool-attempt-*.out", "tool-attempt-*.stderr"):
    for path in glob.glob(os.path.join(logs, pattern)):
        try:
            chars += len(open(path, errors="replace").read())
        except OSError:
            pass
summary = {"estimated_transcript_tokens": math.ceil(chars / 4), "method": "chars_div_4"}
summary = json.dumps(summary, separators=(",", ":"))
print(json.dumps(summary)[1:-1])
PY
)
    ;;
esac
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

valid=true
case "$tool" in
  codex)
    host_shell_without_wrapper=$(grep -h '"command_execution"' "$logs"/tool-attempt-*.jsonl 2>/dev/null | grep -Fvc "$bin_dir/eval-shell" || true)
    if [[ "$host_shell_without_wrapper" -gt 0 ]]; then
      valid=review
    fi
    ;;
  *)
    if [[ "$command_count" -eq 0 ]]; then
      valid=review
    fi
    ;;
esac

printf '{"run_id":"%s","task_id":"%s","tool":"%s","shell_arm":"%s","image":"%s","valid":"%s","started_at":"%s","finished_at":"%s","wall_time_sec":%s,"success":%s,"tool_exit":%s,"verifier_exit":%s,"failure_mode":"%s","tool_call_count":%s,"bash_command_invocations":%s,"retry_count":%s,"retry_sleep_sec":%s,"rate_limit_or_api_error_count":%s,"token_usage_excerpt":"%s","workspace":"%s","log_dir":"%s"}\n' \
  "$run_id" "$task" "$tool" "$env_name" "$image" "$valid" "$started_at" "$finished_at" "$wall_time_sec" "$success" "$tool_exit" "$verifier_exit" "$failure_mode" "$tool_call_count" "$command_count" "$retry_count" "$retry_sleep_sec" "$rate_limit_count" "$token_usage" "$work" "$logs" >>"$out"

summary="$repo/docs/agent-shell-eval/test-$run_id.md"
cat >"$summary" <<SUMMARY
# Agent Shell Evaluation Test: $run_id

- Task: $task
- Agent tool: $tool
- Shell arm: $env_name
- Container image: $image
- Result: success=$success, valid=$valid
- Wall time: ${wall_time_sec}s
- Tool calls: $tool_call_count
- Bash command invocations: $command_count
- Retries: $retry_count
- Retry sleep: ${retry_sleep_sec}s
- Rate-limit/API error signals: $rate_limit_count
- Logs: $logs

## Verifier

\`\`\`text
$(sed -n '1,120p' "$logs/verify.out" 2>/dev/null)
$(sed -n '1,120p' "$logs/verify.stderr" 2>/dev/null)
\`\`\`

## Command Log

\`\`\`text
$(sed -n '1,120p' "$logs/container-shell.tsv" 2>/dev/null)
\`\`\`

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
SUMMARY

printf 'run_id=%s success=%s valid=%s wall_time_sec=%s logs=%s summary=%s\n' "$run_id" "$success" "$valid" "$wall_time_sec" "$logs" "$summary"

if [[ "$success" == true ]]; then
  exit 0
fi
exit 1
