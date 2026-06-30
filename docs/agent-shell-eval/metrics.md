# Metrics

Every evaluation run should record both per-run metrics and aggregate report
metrics.

## Per-Run Metrics

- `run_id`
- `task_id`
- `task_bucket`
- `tool`
- `env`
- `container_image`
- `shell_contract`
- `valid`
- `success`
- `failure_mode`
- `started_at`
- `finished_at`
- `setup_time_sec`
- `agent_time_sec`
- `verify_time_sec`
- `wall_time_sec`
- `native_input_tokens`
- `native_output_tokens`
- `native_cached_input_tokens`
- `native_reasoning_output_tokens`
- `estimated_transcript_tokens`
- `tool_call_count`
- `bash_command_invocations`
- `command_count_total`
- `edit_count`
- `files_changed`
- `retry_count`
- `api_error_count`
- `rate_limit_retry_count`
- `rate_limit_backoff_sec`
- `first_api_error_at`
- `last_api_error_at`
- `intervention_count`
- `rate_limit_count`
- `advisor_hint_count`
- `shell_escape_attempts`
- `destructive_command_attempts`
- `container_cpu_sec`
- `container_max_rss_bytes`
- `verifier_exit`
- `verifier_output_hash`
- `log_dir`

## Aggregate Metrics

- Pass/fail total and pass ratio by tool/env/task bucket.
- Invalid/contaminated run total and ratio.
- Median/p90 wall time, agent time, and verify time.
- Native token totals and estimated transcript token totals, kept separate.
- Tool-call count distribution.
- Bash-command invocation count distribution.
- Retry/recovery count after failed shell commands.
- API/rate-limit error count, retry count, and total backoff duration.
- Human/conductor intervention count.
- Rate-limit/budget failure count.
- Advisor hint count and acted-on-hint count.
- Shell escape attempts.
- Destructive command attempts and whether safety surfaces prevented damage.
- Files changed and edit count.
- Verifier rerun count.
- Regression count against broader guard.
- Container resource profile: max RSS and CPU seconds.

## Approval-Tracked Estimates

Before each run or batch, append an estimate to `run-log.md`:

```text
Estimate:
- tools:
- envs:
- tasks:
- runs:
- wall time:
- token range:
- paid budget exposure:
- approval required:
- approval status:
```

`opencode` and `aider` require explicit user approval before launch because
they consume DeepSeek-backed budget. Batches over 10 total agent runs also
require explicit approval regardless of tool.

## Rate Limits And API Errors

Subscription-backed tools (`codex`, `claude`, `agy`) can still fail or stall due
to rate limits or provider API errors. Treat those as real-life operational
outcomes:

- Record the error text/class when available.
- Record retry count and backoff duration.
- Include backoff time in wall-clock duration.
- Keep agent-time and provider-backoff time separate when possible.
- Do not silently discard a rate-limited run; either resume it or mark the final
  status as rate-limit/API-error.
- Retrying after a reasonable backoff is allowed, but every retry must appear in
  the run log and metrics.
