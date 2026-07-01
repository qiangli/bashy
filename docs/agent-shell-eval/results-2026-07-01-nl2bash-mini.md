# IBM NL2Bash Mini Result - 2026-07-01

## Scope

- Campaign: standard benchmark Batch 1, IBM NL2Bash mini.
- Benchmark source: `~/tests/bashy-standard-benchmarks/nl2bash-eabench`.
- Benchmark revision: `83e4d89af327955e312ca3f375da7363e7c219aa`.
- Dataset license note: prompts are covered by the Community Data License Agreement - Permissive - Version 2.0.
- Selected sample: `bash_1/test1`, `bash_1/test2`, `bash_1/test3`.
- Selection rule: first three simple `bash_1` tasks with portable shell verifiers and no Python/container dependency inside the evaluated shell image.
- Tools: `codex`, `claude`, `agy`.
- Shell arms:
  - `bashy-current`: `bashy-agent-shell:bashy-current`
  - `gnu-bash53`: `bashy-agent-shell:gnu-bash53`, original GNU Bash `5.3.0(2)-release`
- Harness: `eval/agent-shell/run-container-task.sh` with host agents and container-enforced task shell execution via `bin/bashy podman`.
- Raw JSONL: `results/agent-shell-nl2bash-mini.jsonl`.
- Run workspaces/logs: `~/tests/bashy-eval/runs-nl2bash/`.

## Tasks

| Task | Original prompt | Expected behavior |
| --- | --- | --- |
| `nl2bash-basic-test1` | Create a new directory test in the current directory | Write `test1.sh`; running it creates `test/`. |
| `nl2bash-basic-test2` | Copy file test.txt from directory dir1 to dir2 in the current directory | Write `test2.sh`; running it copies `dir1/test.txt` to `dir2/test.txt`. |
| `nl2bash-basic-test3` | Create a new file test.json in the current directory with the content {"name": "test"} | Write `test3.sh`; running it creates JSON containing `"name": "test"`. |

## Headline

All selected valid runs passed:

- Bashy-current: `9/9` passed.
- GNU Bash 5.3: `9/9` passed.
- Total valid matrix: `18/18` passed.
- No retries were needed.
- One harness rate-limit/API signal was counted in a successful Claude/bashy-current run; there was no retry because the tool exited successfully.
- One invalid pre-fix smoke run is retained in raw JSONL and excluded from the headline. It failed because the verifier reran a non-idempotent `mkdir test` script after the agent had already tested it. The verifier now removes generated outputs before the official run.

## Aggregate Metrics

| Shell arm | Runs | Pass | Total wall time | Tool calls | Shell invocations | Retries | API/rate-limit signals |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `bashy-current` | 9 | 9 | 157s | 42 | 30 | 0 | 1 |
| `gnu-bash53` | 9 | 9 | 202s | 57 | 44 | 0 | 0 |

By tool:

| Tool | Shell arm | Pass | Total wall time | Tool calls | Shell invocations |
| --- | --- | ---: | ---: | ---: | ---: |
| `agy` | `bashy-current` | 3/3 | 57s | 14 | 14 |
| `agy` | `gnu-bash53` | 3/3 | 105s | 27 | 27 |
| `claude` | `bashy-current` | 3/3 | 36s | 12 | 5 |
| `claude` | `gnu-bash53` | 3/3 | 49s | 16 | 7 |
| `codex` | `bashy-current` | 3/3 | 64s | 16 | 11 |
| `codex` | `gnu-bash53` | 3/3 | 48s | 14 | 10 |

## Per-Run Metrics

| Task | Tool | Shell arm | Result | Wall | Tool calls | Shell invocations | Retries | API/rate-limit signals | Token/cost excerpt |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| `test1` | `agy` | `bashy-current` | pass | 18s | 4 | 4 | 0 | 0 | `not_parsed` |
| `test1` | `agy` | `gnu-bash53` | pass | 49s | 12 | 12 | 0 | 0 | `not_parsed` |
| `test1` | `claude` | `bashy-current` | pass | 12s | 4 | 2 | 0 | 0 | `total_cost_usd=0.14720999999999998` |
| `test1` | `claude` | `gnu-bash53` | pass | 11s | 4 | 2 | 0 | 0 | `total_cost_usd=0.14918599999999999` |
| `test1` | `codex` | `bashy-current` | pass | 8s | 2 | 2 | 0 | 0 | `{"input_tokens":23639,"cached_input_tokens":19200,"output_tokens":224,"reasoning_output_tokens":0}` |
| `test1` | `codex` | `gnu-bash53` | pass | 7s | 2 | 2 | 0 | 0 | `{"input_tokens":23621,"cached_input_tokens":21248,"output_tokens":180,"reasoning_output_tokens":0}` |
| `test2` | `agy` | `bashy-current` | pass | 18s | 5 | 5 | 0 | 0 | `not_parsed` |
| `test2` | `agy` | `gnu-bash53` | pass | 23s | 5 | 5 | 0 | 0 | `not_parsed` |
| `test2` | `claude` | `bashy-current` | pass | 12s | 4 | 1 | 0 | 0 | `total_cost_usd=0.1504705` |
| `test2` | `claude` | `gnu-bash53` | pass | 24s | 8 | 3 | 0 | 0 | `total_cost_usd=0.20194750000000003` |
| `test2` | `codex` | `bashy-current` | pass | 33s | 10 | 7 | 0 | 0 | `{"input_tokens":74836,"cached_input_tokens":62208,"output_tokens":1083,"reasoning_output_tokens":74}` |
| `test2` | `codex` | `gnu-bash53` | pass | 13s | 4 | 3 | 0 | 0 | `{"input_tokens":36110,"cached_input_tokens":30848,"output_tokens":425,"reasoning_output_tokens":0}` |
| `test3` | `agy` | `bashy-current` | pass | 21s | 5 | 5 | 0 | 0 | `not_parsed` |
| `test3` | `agy` | `gnu-bash53` | pass | 33s | 10 | 10 | 0 | 0 | `not_parsed` |
| `test3` | `claude` | `bashy-current` | pass | 12s | 4 | 2 | 0 | 1 | `total_cost_usd=0.1495445` |
| `test3` | `claude` | `gnu-bash53` | pass | 14s | 4 | 2 | 0 | 0 | `total_cost_usd=0.14819400000000002` |
| `test3` | `codex` | `bashy-current` | pass | 23s | 4 | 2 | 0 | 0 | `{"input_tokens":48618,"cached_input_tokens":42496,"output_tokens":794,"reasoning_output_tokens":170}` |
| `test3` | `codex` | `gnu-bash53` | pass | 28s | 8 | 5 | 0 | 0 | `{"input_tokens":74212,"cached_input_tokens":65280,"output_tokens":963,"reasoning_output_tokens":134}` |

## Interpretation

This mini is too small and too easy to claim broad superiority. It is still useful because it validates the container-enforced benchmark loop against a real published NL2Bash execution benchmark and shows that bashy-current did not hurt task success on simple command/script-generation tasks.

The strongest signal in this batch is reduced interaction volume for `agy` and `claude` on bashy-current:

- `agy`: bashy-current used 14 shell invocations vs. 27 under GNU Bash.
- `claude`: bashy-current used 5 shell invocations vs. 7 under GNU Bash.
- `codex`: roughly neutral, with GNU slightly faster in this small sample.

The total bashy-current wall time was lower overall (`157s` vs. `202s`), but the sample is too small and agent nondeterminism is too high to treat that as a stable performance claim.

## Harness / Product Findings

- The verifier must tolerate agents testing their own generated script before final verification. The NL2Bash adapters now remove generated outputs before rerunning the script.
- `agy` token usage is still not parsed, so cross-tool token reporting remains incomplete.
- The rate-limit/API signal detector likely over-counted one successful Claude run; the log should be inspected before treating that as a real provider issue.
- The batch did not stress bashy's agentic features much. The next NL2Bash slice should include tasks where bashy shell-check/commands/advisor surfaces can plausibly matter.

## Next Benchmark Step

Run an IBM NL2Bash system-oriented mini using `bash_2`, but filter out privileged/system-mutating tasks. Good candidates are read-only diagnostics such as disk free space, process listing, free memory, uptime, or file listing tasks. Keep the next batch under 30 minutes and continue excluding `opencode`/`aider` until approved.
