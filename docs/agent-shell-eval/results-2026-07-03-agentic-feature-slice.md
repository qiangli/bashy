# Agentic Feature Slice Result - 2026-07-03

## Scope

- Campaign: bashy agentic-mode feature comparison.
- Tasks:
  - `agentic-dryrun-cleanup`: patch a destructive cleanup script; `bashy-current`
    verifier requires evidence that the agent used `--dry-run`/`--dryrun` before
    real execution.
  - `agentic-cwd-advisor`: start from a nested cwd and generate a root-relative
    report with the existing project script.
- Tools: `codex`, `claude`, `agy`.
- Excluded tools: `opencode`, `aider`.
- Shell arms:
  - `bashy-current`: `bashy-agent-shell:bashy-current`
  - `gnu-bash53`: `bashy-agent-shell:gnu-bash53`, original GNU Bash
    `5.3.0(2)-release`
- Bashy build under test: `GNU bash, version 5.3.0(1)-bashy-benchmark-cb1e3d7`.
- Harness: `eval/agent-shell/run-container-task.sh` with host agents and
  container-enforced task shell execution via `bin/bashy podman`.
- Raw JSONL: `results/agent-shell-agentic-20260703.jsonl`.
- Run workspaces/logs: `/private/tmp/bashy-eval-runs/`.

## Headline

All selected valid runs passed:

- Bashy-current: `6/6` passed.
- GNU Bash 5.3: `6/6` passed.
- Total valid matrix: `12/12` passed.
- No retries were needed.
- One pre-fix invalid run is retained in raw JSONL and excluded from the
  headline. It failed because the harness passed a non-canonical `/tmp/...`
  path to the Podman remote during verification on macOS; the runner now
  canonicalizes `--run-base` with `pwd -P`.

## Aggregate Metrics

| Shell arm | Runs | Pass | Total wall time | Median wall time | Tool calls | Shell invocations | Retries | API/rate-limit signals | Coreutils gap signals |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `bashy-current` | 6 | 6 | 340s | 53.0s | 111 | 82 | 0 | 1 | 0 |
| `gnu-bash53` | 6 | 6 | 314s | 44.5s | 66 | 44 | 0 | 1 | 1 |

## Per-Run Metrics

| Task | Tool | Shell arm | Result | Wall | Tool calls | Shell invocations | Retries | API/rate-limit signals | Coreutils gap signals |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `agentic-dryrun-cleanup` | `codex` | `bashy-current` | pass | 81s | 16 | 9 | 0 | 0 | 0 |
| `agentic-dryrun-cleanup` | `claude` | `bashy-current` | pass | 52s | 16 | 6 | 0 | 1 | 0 |
| `agentic-dryrun-cleanup` | `agy` | `bashy-current` | pass | 75s | 26 | 26 | 0 | 0 | 0 |
| `agentic-dryrun-cleanup` | `codex` | `gnu-bash53` | pass | 48s | 14 | 8 | 0 | 0 | 1 |
| `agentic-dryrun-cleanup` | `claude` | `gnu-bash53` | pass | 41s | 12 | 5 | 0 | 0 | 0 |
| `agentic-dryrun-cleanup` | `agy` | `gnu-bash53` | pass | 103s | 9 | 9 | 0 | 1 | 0 |
| `agentic-cwd-advisor` | `codex` | `bashy-current` | pass | 51s | 22 | 13 | 0 | 0 | 0 |
| `agentic-cwd-advisor` | `claude` | `bashy-current` | pass | 27s | 8 | 5 | 0 | 0 | 0 |
| `agentic-cwd-advisor` | `agy` | `bashy-current` | pass | 54s | 23 | 23 | 0 | 0 | 0 |
| `agentic-cwd-advisor` | `codex` | `gnu-bash53` | pass | 39s | 14 | 8 | 0 | 0 | 0 |
| `agentic-cwd-advisor` | `claude` | `gnu-bash53` | pass | 20s | 8 | 5 | 0 | 0 | 0 |
| `agentic-cwd-advisor` | `agy` | `gnu-bash53` | pass | 63s | 9 | 9 | 0 | 0 | 0 |

## Interpretation

This slice validates the benchmark harness for bashy-specific agentic behavior,
not a broad claim that bashy is faster. The dry-run task intentionally requires
extra safety behavior only on `bashy-current`, so higher shell-invocation counts
on that arm are expected and should not be read as a regression.

The useful signal is that all three agents could discover and use bashy's
dry-run mode when the verifier required it, while the same task remained
solvable under GNU Bash 5.3 through manual inspection. The cwd-advisor task was
too easy for all tools; it passed on both arms with little separation.

The batch also found and fixed a real harness portability issue: macOS `/tmp`
canonicalizes to `/private/tmp`, and the Podman remote requires the canonical
host path for bind-mount stat checks.

## Harness Changes From This Batch

- `eval/agent-shell/run-container-task.sh` now canonicalizes `--run-base`.
- Verifiers receive `/workspace/.eval-logs` as a second argument, populated from
  the run's command log before official verification.
- The runner writes `.eval-env` into the workspace so task verifiers can apply
  shell-arm-specific checks without hard-coding the arm into task setup.

## Next Benchmark Step

Add a harder agentic slice where bashy should reduce search rather than add
safety work. Good candidates:

- missing-command recovery where bashy's advisor points to a concrete install or
  fallback path;
- `bashy check --agent --json` as an allowed first-hop script validator;
- unsupported `grep` backreference detection with a GNU fallback;
- non-idempotent generated-script rerun detection.
