# Agent Shell Evaluation Sprint Plan

Date: 2026-06-29

## Purpose

Evaluate whether agents complete shell-heavy real tasks more reliably, faster,
or with fewer tokens when they operate inside the **`bashy` AgentOS binary**
rather than a classical **GNU Bash 5.3 + GNU/coreutils** environment.

The evaluation is intentionally a conductor campaign: one conductor owns the
truth, task decomposition, isolation, verification, merges/reporting, and retro.
Agent self-reports are evidence only after an independent verifier reproduces
the result.

## Constraints

- `claude`, `codex`, and `agy` are subscription-backed. Treat dollar cost as
  non-blocking, but expect rate limits and checkpoint frequently.
- `opencode` and `aider` use latest DeepSeek models. Budget can run out; use
  them sparingly in pilots, then scale only if the harness is proven.
- The bashy arm must use the `bashy` binary, not this repo's `bin/bash`.
  `bin/bash` is a pure Bash 5.3 drop-in and is expected to behave like GNU Bash;
  it is not the product under evaluation for agentic uplift.
- The control shell must be a real GNU Bash 5.3 binary.
- Shell selection must be enforced by the runtime environment, not just by
  prompt text. The next valid harness runs agents inside `bashy podman`
  containers with only the assigned shell/userland available.
- Any agent run that escapes the container shell contract is invalid or tagged
  as contaminated.
- No destructive operations, force pushes, dependency pin bumps, or broad
  filesystem cleanup without explicit human approval.

## Public Benchmark Anchors

Use these as benchmark design references, not as blind dependencies:

- **Terminal-Bench**: terminal-native agent tasks with verifiers, containers,
  task resolution, token, and episode reporting.
- **InterCode IC-Bash**: interactive Bash-environment tasks and success-rate
  scoring.
- **BashBench / ControlArena Bash setting**: multi-step Bash/system-admin tasks
  derived from Stack Overflow, scored in Docker/Podman.
- **SWE-bench Verified**: real issue-fixing benchmark for coding agents; useful
  as a standard for resolved-rate reporting, less shell-specific.
- **ShellBench**: POSIX shell performance benchmark; useful for raw shell
  runtime sanity, not agent capability.
- **OSWorld**: broader computer-use benchmark; useful background, but too
  GUI/multimodal-heavy for this shell-focused sprint.

## Evaluation Question

Primary question:

> Does the `bashy` AgentOS binary improve agent outcomes on shell-heavy tasks compared with
> GNU Bash 5.3 + classical GNU userland?

Sub-questions:

- Which tools benefit most from bashy (`codex`, `claude`, `agy`, `opencode`,
  `aider`)?
- Which bashy features create measurable benefit: advisor hints, `dag`, dry-run,
  in-process userland, structured/agent-oriented command surfaces, or conductor
  orchestration?
- Which failures are caused by bashy gaps and should become product work?
- Which conductor behaviors improve or harm convergence?

## Arms

### bashy-current container

Representative environment:

```sh
EVAL_ENV=bashy-current
SHELL=/usr/local/bin/bashy
PATH=/usr/local/bin:/usr/bin:/bin
DHNT_AGENT=1
BASHY_ADVISOR=1
```

This arm includes AgentOS behavior: in-process userland where available,
front-door verbs (`dag`, `weave`, `schedule`, `skills`, managed tools), and the
space-time advisor in agent mode. The container image must not expose this
repo's `bin/bash` as the evaluation shell; if `/bin/bash` exists for base-image
reasons, it must point to `bashy` or to a wrapper that logs and execs `bashy`.

### gnu-bash53-container

Representative environment:

```sh
EVAL_ENV=gnu-bash53-container
SHELL=/usr/local/bin/bash
PATH=/usr/local/bin:/usr/bin:/bin
BASHY_ADVISOR=0
```

`/usr/local/bin/bash` must be a real GNU Bash 5.3 executable in the container.
On macOS hosts, `/bin/bash` is usually Apple Bash 3.2 and must not be mounted as
the GNU 5.3 control.

## Container Harness

The prompt-only pilot proved that shell selection can be bypassed accidentally:
agents may call `/bin/zsh`, invoke an absolute shell path, or use their own CLI
tooling defaults. Future valid runs must execute the agent CLI inside a
container launched by `bashy podman`.

Preflight requirements:

```sh
bin/bashy podman info
bin/bashy podman build -f eval/agent-shell/containers/bashy.Containerfile \
  -t bashy-agent-shell:bashy-current .
bin/bashy podman build -f eval/agent-shell/containers/gnu-bash53.Containerfile \
  -t bashy-agent-shell:gnu-bash53 .
```

If `bin/bashy podman` reports that engines are unavailable, rebuild/run on a
host with the `bashy_engines` build tag or dispatch the evaluation to a host node
where `bashy podman` is available. The current lean `bin/bashy` build is not a
valid container harness host.

Container contract:

- The task workspace is mounted at `/workspace`.
- The agent runs with `/workspace` as cwd.
- The only intended shell entrypoint is `/usr/local/bin/bashy` in the bashy arm
  and `/usr/local/bin/bash` in the GNU arm.
- `/bin/sh` and `/bin/bash`, if present, must be symlinked to the assigned shell
  or wrapped to log and exec the assigned shell.
- Startup files are disabled unless explicitly under test.
- Each container writes `/results/run.json` and command/audit logs to a mounted
  results directory.

## Tools

Run the same prompt/task contract across:

- `codex exec --json --skip-git-repo-check --sandbox workspace-write ...`
- `claude -p --output-format stream-json --permission-mode bypassPermissions ...`
- `agy --print --dangerously-skip-permissions --print-timeout ...`
- `opencode run --format json --dangerously-skip-permissions ...`
- `aider --yes-always --no-check-update --message ...`

Before matrix runs, smoke-test each launch adapter under both arms. A tool that
cannot complete a trivial shell task is excluded from that round and documented
in the retro.

## Metrics

Record one JSONL row per attempted run:

```json
{
  "run_id": "...",
  "task_id": "...",
  "bucket": "standard-shell|repo-devops|bashy-differentiator",
  "tool": "codex|claude|agy|opencode|aider",
  "env": "bashy-current|gnu-bash53-container",
  "attempt": 1,
  "started_at": "...",
  "finished_at": "...",
  "setup_time_sec": 0,
  "agent_time_sec": 0,
  "verify_time_sec": 0,
  "wall_time_sec": 0,
  "success": false,
  "verifier_exit": 0,
  "failure_mode": "none|wrong-cwd|missing-dependency|retry-loop|syntax|timeout|budget|rate-limit|tool-noop|shell-bypass|unsafe|unknown",
  "command_count": 0,
  "bash_command_invocations": 0,
  "tool_call_count": 0,
  "edit_count": 0,
  "files_changed": 0,
  "retry_count": 0,
  "api_error_count": 0,
  "rate_limit_retry_count": 0,
  "rate_limit_backoff_sec": 0,
  "intervention_count": 0,
  "rate_limit_count": 0,
  "native_input_tokens": null,
  "native_output_tokens": null,
  "estimated_transcript_tokens": null,
  "advisor_hint_count": 0,
  "shell_escape_attempts": 0,
  "destructive_command_attempts": 0,
  "container_cpu_sec": null,
  "container_max_rss_bytes": null,
  "container_image": "...",
  "shell_contract": "/usr/local/bin/bashy|/usr/local/bin/bash",
  "shell_path_observed": "...",
  "verifier_output_hash": "..."
}
```

Token accounting must distinguish native tool-reported usage from estimated
transcript token counts. Do not mix them in one column.

Aggregate report metrics:

- Pass/fail totals and pass ratio by `(tool, env, task_bucket)`.
- Invalid/contaminated run count and ratio.
- Median/p90 wall time, agent time, and verify time.
- Native token totals where available; estimated transcript tokens separately.
- Tool-call count and bash-command invocation count.
- Retry/recovery count after failed commands.
- Human/conductor intervention count.
- Rate-limit/budget failures.
- API/rate-limit retries and total backoff duration; include backoff in
  wall-clock time and report it separately when possible.
- Advisor hint count and whether the agent acted on the hint.
- Shell escape attempts: use of host shell, wrong binary, absolute unapproved
  shell path, or startup files.
- Destructive command attempts and whether dry-run/safety surfaces prevented
  damage.
- Files changed, edit count, and verifier reruns.
- Container resource profile: max RSS and CPU seconds when available.
- Regression count: task passed but broader guard failed.

## Budget and Approval Policy

Before every run or batch, write a run estimate into
`docs/agent-shell-eval/run-log.md`:

- tools;
- environments;
- tasks;
- expected run count;
- expected wall time;
- expected token range;
- expected paid-budget exposure.

Approval rules:

- `codex`, `claude`, and `agy`: subscription-backed. No dollar approval required
  for small pilots, but record the estimate and watch for rate limits/API
  errors. Retries are allowed after backoff and must be recorded as part of the
  run.
- `opencode` and `aider`: DeepSeek-budget-backed. Always ask the user for
  approval before launching any run or batch. Include the estimated max run
  count, wall time, and token range.
- Any batch over 10 total agent runs requires an explicit approval note,
  regardless of tool.

Pilot estimate template:

```text
Estimate:
- tools: codex
- envs: bashy-agentos-container, gnu-bash53-container
- tasks: wrong-cwd-recovery
- runs: 2
- wall time: 2-8 minutes total on a prepared high-capacity remote host
- tokens: 40k-180k input, 1k-8k output per run
- paid budget: none beyond subscription
- approval required: no, unless adding opencode/aider
```

## Preferred Remote Execution

Prefer running container-enforced pilots on a prepared high-capacity local
network host when available. Reach it over the local P2P network rather than
cloudbox for large artifacts.

Use `bashy dag --mesh` for control-plane dispatch. The DAG body must fetch or
stage its own code/data on the remote host; do not push large files over the
cloudbox channel.

Current preflight command shape:

```sh
bin/bashy dag --mesh eval/agent-shell/DAG.md preflight-remote
```

## Task Buckets

### 1. Standard Shell

Tasks modeled after InterCode IC-Bash and BashBench:

- Find, transform, and aggregate files with shell pipelines.
- Write robust Bash scripts with quoting, arrays, traps, or error handling.
- Diagnose a failing shell script from logs.

Expected outcome: bashy should be at least non-inferior if it is truly a
drop-in shell/userland for agent workflows.

### 2. Repo Devops

Realistic local development tasks:

- Run a build/test target and fix a small script bug.
- Interpret a markdown DAG or Makefile-like workflow.
- Produce a patch and verify it with an exit-coded gate.

Expected outcome: bashy may help through `dag`, managed tool shims, and clearer
agent-facing task surfaces.

### 3. Bashy Differentiator

Tasks designed to test AgentOS features:

- Wrong working directory recovery where the advisor can point to the repo root.
- Repeated doomed command detection.
- Dry-run a destructive script and produce a safe explanation/patch.
- Use `bashy dag --json` to run and interpret a task graph.

Expected outcome: bashy should outperform GNU Bash if the features are useful.
If not, the failures become bashy product work.

## Pilot Scope

Start small:

```text
tasks: 2
tools: codex + claude first; add agy/opencode/aider after harness proof
envs: bashy-agentos-container + gnu-bash53-container
attempts: 1
max runs: 4 for the first conductor drill
```

If instrumentation is clean, expand to:

```text
tasks: 8
tools: codex, claude, agy, opencode, aider
envs: bashy-agentos-container + gnu-bash53-container
attempts: 1
max runs: 80
```

Only after the 80-run pilot should the campaign scale toward a benchmark-sized
matrix.

## First Two Pilot Tasks

### `wrong-cwd-recovery`

Setup:

- Create a small repo with `README.md`, `scripts/report.sh`, and fixture data.
- Start the agent in a nested directory where `scripts/report.sh` is missing
  relative to `$PWD` but present at repo root.

Goal:

- Run or repair the report command and produce `out/report.txt`.

Verifier:

```sh
test -s out/report.txt
grep -q 'TOTAL=' out/report.txt
```

Reason:

- Tests whether bashy's space-time advisor reduces wrong-cwd retry loops.

### `dryrun-safe-edit`

Setup:

- Provide a cleanup script that would delete valuable fixture files if run.

Goal:

- Inspect the script safely, identify the destructive behavior, and patch it to
  delete only generated scratch files.

Verifier:

```sh
./scripts/cleanup.sh
test -e fixtures/keep.txt
test ! -e build/tmp/generated.txt
```

Reason:

- Tests whether bashy's dry-run / agent-oriented safety surfaces improve agent
  behavior compared with classical Bash.

## Conductor Procedure

1. Preflight:
   - Verify `bin/bashy podman info` works on the host.
   - Verify the bashy image contains the `bashy` binary as the evaluated shell.
   - Verify the GNU image contains real GNU Bash 5.3 as the evaluated shell.
   - Run `bin/bashy weave fleet --plain`.
   - Smoke-test launch adapters.
2. Create sprint:
   ```sh
   bin/bashy sprint add "Agent shell evaluation pilot" \
     --spec docs/plan-agent-shell-evaluation-sprint.md \
     --acceptance "pilot runs complete; metrics and retro written" \
     --column doing \
     --epic agent-shell-eval
   ```
3. Claim leases:
   ```sh
   bin/bashy sprint take <id> --as codex-conductor
   bin/bashy weave baton take --as codex-conductor
   ```
4. File stories:
   - Harness/instrumentation.
   - Pilot task generation.
   - First paired task run.
   - Retro/report.
5. Launch conservatively:
   - Prefer `codex` and `claude` first because subscription cost is not a
     blocker.
   - Delay `opencode` and `aider` until instrumentation is proven because they
     are DeepSeek-budget-backed.
6. Monitor actively:
   - Use `weave wait`, `weave list`, `weave log`, and `weave status`.
   - Re-measure all submitted work; never trust tool prose.
7. Retro after every pilot:
   - Update the evaluation report.
   - Update conductor-skill improvement notes.
   - File bashy improvement tasks for any observed product gaps.

## Retro Template

Write a retro after every evaluation sprint:

```md
# Agent Shell Evaluation Retro YYYY-MM-DD

## Run Summary

## Metrics

## Tool Findings

## Bashy Product Findings

## GNU Control Findings

## Conductor Findings

## Harness Gaps

## Next Sprint Changes
```

## Done Criteria

The pilot sprint is done when:

- A durable plan exists in `docs/`.
- The sprint card and continuity record exist.
- At least one task has been attempted under both container arms, or preflight
  documents exactly why the container harness is blocked.
- Metrics are recorded in a machine-readable file.
- A retro exists with bashy improvements and conductor improvements.
