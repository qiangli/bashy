# IBM NL2Bash Mini Retro - 2026-07-01

## Run Summary

Batch 1 ran a small IBM NL2Bash execution-accuracy matrix:

- 3 tasks from `bash_1`.
- 3 tools: `codex`, `claude`, `agy`.
- 2 shell arms: `bashy-current`, `gnu-bash53`.
- 18 selected valid runs.

Result: `18/18` selected valid runs passed. The raw JSONL includes one invalid pre-fix smoke failure caused by a verifier-idempotency defect, not an agent or shell failure.

## What Worked

- The existing `eval/agent-shell/run-container-task.sh` harness was good enough to adapt a real external benchmark quickly.
- `bin/bashy podman` enforced the compared shell arm inside containers while allowing host agent tools to run normally.
- The small batch completed well below the 30-minute cap.
- The raw JSONL plus per-run markdown summaries are enough to reconstruct wall time, pass/fail, tool calls, shell invocations, retries, and partial token/cost data.
- Bashy-current had no failures on the selected NL2Bash tasks.

## What Should Improve

- NL2Bash adapters should be generated from the upstream benchmark metadata instead of hand-authored per selected task. The adapter generator should copy prompt/setup fixtures and synthesize a portable verifier where possible.
- Verifiers must assume agents may run the candidate script during development. Official verification should reset generated outputs before rerunning.
- `agy` token usage remains unparsed.
- Rate-limit/API detection is too broad. One successful Claude run was counted with an API/rate-limit signal even though there was no retry and no task failure.
- The first Claude weave delegation failed because the conductor launch command did not provide prompt text to `claude -p`. Weave issue #1 in `../ycode` failed immediately with: `Input must be provided either through stdin or as a prompt argument when using --print`.

## Bashy Product Findings

- For simple NL2Bash tasks, bashy-current is already compatible enough to complete the same generated scripts as GNU Bash 5.3.
- The current sample did not exercise bashy's differentiators strongly. Future batches should include tasks where `bashy check`, `bashy commands`, advisor output, missing-command detection, or dry-run behavior can change the agent's search path.
- The result still supports the evaluation premise: bashy did not reduce correctness and appears to reduce shell-command churn for some tools in this small sample.

## How Bashy Can Outperform GNU Bash 5.3 More

The batch shows a useful but weak advantage: bashy-current reduced total shell
invocations (`30` vs. `44`) and total wall time (`157s` vs. `202s`) while keeping
the same pass rate. To widen that gap, bashy needs to make the first shell
interaction more informative so agents stop probing.

Highest-impact improvements:

- **Make `bashy check` the agent's first move.** Add a compact agent-readable
  mode such as `bashy check --agent --script test.sh --cwd .` that reports
  syntax validity, required commands, whether each command is built in / embedded
  coreutils / external PATH / missing, and likely portability issues. Expected
  metric movement: fewer exploratory `ls`, `cat`, `which`, `bash -n`, and retry
  commands.
- **Expose a one-shot script validation envelope.** Add or document a command
  like `bashy run --check --json ./candidate.sh` that combines parse, dry-run
  command inventory, and execution result. Agents currently spend separate tool
  calls doing chmod, syntax check, run, inspect output. Expected metric movement:
  lower tool-call count and shell-command count on NL2Bash/script-writing tasks.
- **Make generated-script workflows idempotency-aware.** In agent mode, when a
  script fails with common stateful messages such as `File exists`, `No such file
  or directory`, or destination already exists, bashy should print a single
  advisory hint explaining whether the failure likely came from rerunning a
  non-idempotent script. Expected metric movement: fewer wasted retries and fewer
  false verifier failures.
- **Improve command/coreutils capability surfacing.** `bashy commands` should be
  easy for agents to query by command name and flag, for example
  `bashy commands grep --json --features`. The Koala smoke showed `grep`
  backreferences as a concrete gap; agents should be told before they choose an
  unsupported path. Expected metric movement: fewer failed runs on text-processing
  benchmarks and better fallback choices.
- **Tighten built-in safety affordances.** The earlier dry-run benchmark showed
  value. Make `--dryrun` and destructive-operation summaries more discoverable in
  `bashy -h`, `bashy commands`, and agent-mode advisor output. Expected metric
  movement: fewer risky command executions and faster convergence on editing
  tasks.
- **Keep GNU compatibility invisible.** Any advantage disappears if agents must
  debug bashy differences. Continue treating GNU Bash 5.3 compatibility and POSIX
  conformance as hard gates before claiming agentic improvements.

Product backlog from this retro:

1. Add an agent-readable `bashy check --agent --json` script validator and wire
   it into docs/help.
2. Add `bashy run --check --json` or equivalent one-shot validation envelope.
3. Extend the space-time advisor with stateful rerun/idempotency hints.
4. Extend `bashy commands` to report per-command feature gaps and fallback
   behavior.
5. Add benchmark tasks that explicitly reward these features: missing command,
   unsupported grep regex, non-idempotent script rerun, destructive cleanup, and
   wrong cwd.

## Implemented Before Next Batch

- Added `bashy check --agent --script PATH [--cwd DIR]` as the documented
  agent-readable JSON script validator. It reports syntax, recursive script
  inventory, bashy-native commands, system PATH commands, container-fallback
  candidates, dynamic commands, and not-found errors.
- Added `bashy run --check --capture -- SCRIPT` so an agent can get preflight
  plus execution in one structured `bashy-run-v1` envelope.
- Added `bashy commands COMMAND --features` for one-command discovery. `grep`
  now reports the known RE2/backreference gap with an agent hint to use a GNU
  fallback/container for those patterns.
- Updated `bashy help`, `bashy commands --agentic`, and command help text so the
  new surfaces are first-hop discoverable.
- Improved the benchmark harness token/cost parser:
  - Codex keeps structured token usage.
  - Claude now records input/output/cache tokens and model usage in addition to
    cost.
  - Agy now records a conservative transcript-token estimate (`chars / 4`) until
    native usage output is available.
- Tightened rate-limit/API detection so Claude warning-only
  `allowed_warning` rate-limit events are not counted as API failures.

## Conductor Retro

- Do a one-arm smoke before every benchmark batch. It caught the non-idempotent verifier before the whole matrix ran.
- After any harness fix, mark the affected previous run as invalid in the report rather than deleting raw data.
- When delegating through weave, verify the exact agent CLI invocation with a 5-second dry launch or `--no-spawn` equivalent. For Claude, provide a prompt argument or stdin; `claude -p` alone is not a valid worker launch.
- Keep benchmark work and delegated bug-fix work independent. The failed ycode delegation did not block the benchmark batch.

## Follow-Ups

- Add an NL2Bash adapter generator that can select deterministic samples and emit task dirs under `eval/agent-shell/tasks/generated/`.
- Add parser support for `agy` token/cost usage, or at minimum transcript-token estimates.
- Tighten the API/rate-limit grep so normal successful transcript text does not increment the counter.
- Run IBM NL2Bash `bash_2` read-only diagnostic mini next.
