# Agent Shell Eval Results: 2026-06-30 Container Pilot

This pilot compared `bashy v0.4.0` against `GNU Bash 5.3` using the same
agent clients: `codex`, `claude`, and `agy`.

Execution model:

- Agent clients ran on the host.
- Task shell execution was forced through `bin/bashy podman run`.
- The compared shell arm was the container image:
  - `bashy-agent-shell:bashy-v0.4.0`
  - `bashy-agent-shell:gnu-bash53`
- Each run used a disposable workspace under `$RUN_ROOT/runs`.
- `opencode` and `aider` were not run.

## Result Matrix

All 12 valid product-comparison runs passed verification.

| Task | Tool | Shell arm | Result | Wall time | Tool calls | Bash invocations | Retries | Retry sleep | API/rate-limit errors |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| wrong-cwd-recovery | codex | bashy v0.4.0 | pass | 44s | 18 | 10 | 0 | 0s | 0 |
| wrong-cwd-recovery | codex | GNU Bash 5.3 | pass | 40s | 14 | 8 | 0 | 0s | 0 |
| wrong-cwd-recovery | claude | bashy v0.4.0 | pass | 23s | 8 | 5 | 0 | 0s | 0 |
| wrong-cwd-recovery | claude | GNU Bash 5.3 | pass | 28s | 8 | 5 | 0 | 0s | 0 |
| wrong-cwd-recovery | agy | bashy v0.4.0 | pass | 55s | 6 | 6 | 0 | 0s | 0 |
| wrong-cwd-recovery | agy | GNU Bash 5.3 | pass | 55s | 5 | 5 | 0 | 0s | 0 |
| dryrun-safe-edit | codex | bashy v0.4.0 | pass | 25s | 4 | 3 | 0 | 0s | 0 |
| dryrun-safe-edit | codex | GNU Bash 5.3 | pass | 40s | 10 | 6 | 0 | 0s | 0 |
| dryrun-safe-edit | claude | bashy v0.4.0 | pass | 43s | 12 | 13 | 0 | 0s | 0 |
| dryrun-safe-edit | claude | GNU Bash 5.3 | pass | 36s | 12 | 19 | 0 | 0s | 0 |
| dryrun-safe-edit | agy | bashy v0.4.0 | pass | 367s | 41 | 41 | 0 | 0s | 0 |
| dryrun-safe-edit | agy | GNU Bash 5.3 | pass | 3680s | 16 | 16 | 1 | 30s | 1 |

## Aggregates

| Shell arm | Valid runs | Passes | Pass rate | Total wall time | Median wall time | Tool calls | Bash invocations | Retries |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| bashy v0.4.0 | 6 | 6 | 100% | 557s | 43.5s | 89 | 78 | 0 |
| GNU Bash 5.3 | 6 | 6 | 100% | 3879s | 40.0s | 65 | 59 | 1 |

Interpretation:

- On pass/fail, this pilot does not separate the shells: both arms passed 6/6.
- On wall-clock time, bashy was faster in aggregate, but most of that difference
  is the `agy`/GNU timeout-and-retry outlier. Without that outlier, the arms are
  close on these small tasks.
- On command count, bashy sometimes increased exploration because agents probed
  for bashy-specific features. That is useful product feedback, not a conclusive
  negative result.

## Notable Run Behavior

- `codex` used the wrapper consistently and completed both tasks on both arms.
- `claude` required adapter fixes before valid runs:
  - prompt must be passed on stdin because `--add-dir` is variadic;
  - `--output-format stream-json` requires `--verbose`.
- `agy` stdin prompt mode was required; positional prompt mode was unreliable.
- `agy` on `dryrun-safe-edit` with bashy spent several minutes discovering the
  dry-run surface, including trying `--dry-run`, `--dryrun`, `set -o dryrun`,
  and binary-string inspection.
- `agy` on `dryrun-safe-edit` with GNU Bash 5.3 hit `Error: timeout waiting for
  response`, waited 30s, retried once, and eventually passed.
- Claude stream JSON emitted `rate_limit_event` telemetry with
  `status:"allowed"`; these are not counted as errors in the curated table.

## Harness Incidents

These are excluded from product comparison:

- A first Codex/bashy run passed task work but failed verification because the
  harness invoked an entrypoint-shell image as if `/bin/bash` were a script.
- Two early Claude/bashy attempts failed due adapter invocation errors.
- Raw `results/agent-shell-container.jsonl` has malformed lines for some Claude
  runs because the first harness version embedded large nested stream JSON into a
  JSON string. Per-run markdown summaries and this curated result file are the
  source of truth for the pilot. The runner now stores a compact token/cost
  summary for future runs.

## Product Changes From The Pilot

- Fixed `bashy -lc` / `bashy -l -c` compatibility in `internal/cli`.
- Added `--dry-run` as a bashy-only alias for the existing `--dryrun` feature,
  because agents naturally tried the hyphenated spelling first.

These changes are local and not pushed.

