# Benchmark Coreutils Gap Log

Purpose: track missing or incomplete bashy userland support observed during
benchmark runs. The goal is to fix gaps that actually affect agent tasks, then
rerun the same benchmark slice to confirm fewer failures, retries, tool calls,
or shell invocations.

## Recording Policy

Record a gap when a benchmark run exposes one of these classes:

- `missing-command`: command is not available in bashy native userland, managed
  fallback, or the selected environment.
- `missing-option`: command exists but rejects a flag or option used by the
  task.
- `unsupported-regex-feature`: command exists but rejects a regex feature such
  as GNU grep BRE/ERE backreferences.
- `behavior-diff`: command accepts the invocation but output, exit status, or
  side effects differ from GNU behavior.
- `needs-container-fallback`: native support is intentionally incomplete and a
  managed GNU fallback should be used.

The container benchmark harness writes structured detections to:

- per-run: `~/tests/bashy-eval/.../logs/coreutils-gaps.jsonl`
- campaign: `results/agent-shell-coreutils-gaps.jsonl`

Schema:

```json
{
  "schema_version": "bashy-benchmark-gap-v1",
  "run_id": "20260701T...",
  "task_id": "benchmark-task",
  "tool": "codex|claude|agy|opencode|aider",
  "shell_arm": "bashy-current|gnu-bash53",
  "kind": "missing-command|missing-option|unsupported-regex-feature|behavior-diff|needs-container-fallback",
  "command": "grep",
  "option": "--flag-if-any",
  "source": "verify.stderr",
  "line": 1,
  "message": "original log line"
}
```

## Known Gaps From Completed Batches

| Source | Kind | Command | Detail | Next action |
| --- | --- | --- | --- | --- |
| Koala `nfa-regex` smoke | `unsupported-regex-feature` | `grep` | RE2-backed grep rejects BRE backreferences such as `\\1`. | Add GNU-compatible backreference support or route these patterns to managed GNU grep fallback. |

## Fix Policy

After each benchmark batch:

1. Sort observed gaps by benchmark impact: failures first, then retry/tool-call
   cost, then warnings.
2. Add a focused regression test before or with the implementation.
3. Prefer native bashy/coreutils support for common behavior.
4. Use managed GNU fallback for rare or heavyweight GNU compatibility features.
5. Rerun the affected benchmark slice and record whether pass rate, wall time,
   token usage, tool calls, or shell invocations improved.
