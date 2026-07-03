# Standard Benchmark Campaign

Date: 2026-07-01

This campaign extends the bashy-vs-GNU Bash evaluation from local pilot tasks
to known benchmark suites. The goal is to try standard tests in small,
repeatable batches, keep each batch under roughly 30 minutes, write a retro
after each batch, and turn bashy shortcomings into product work before moving
to the next batch.

## Benchmark Sources

Primary agent benchmarks:

- Terminal-Bench: terminal-native agent tasks with task instructions,
  container sandboxes, test scripts, and oracle solutions.
  Source: https://github.com/harbor-framework/terminal-bench
- InterCode IC-Bash: interactive Bash environment for code agents.
  Source: https://github.com/princeton-nlp/intercode
- BashBench / ControlArena Bash setting: Bash/system-administration tasks
  derived from Stack Overflow, with Docker/Podman sandboxing and programmatic
  scoring.
  Source: https://control-arena.aisi.org.uk/settings/bash.html
- IBM NL2Bash Execution Accuracy Benchmarks: 150 Bash generation tests split
  into `bash_1`, `bash_2`, and `bash_3`.
  Source: https://github.com/IBM/nl2bash-eabench

Supporting shell/runtime benchmarks:

- ShellBench: POSIX shell performance comparison utility.
  Source: https://github.com/shellspec/shellbench
- Koala: real-world shell program benchmark suite for POSIX shell
  characterization.
  Source: https://github.com/kbensh/koala

## Campaign Rules

- Keep each batch below 30 minutes wall time unless explicitly approved.
- Use `codex`, `claude`, and `agy` only by default.
- Do not run `opencode` or `aider` without a separate estimate and approval.
- Prefer container-enforced shell arms:
  - `bashy-agent-shell:bashy-current`
  - `bashy-agent-shell:gnu-bash53`
- Record pass/fail, wall time, tool calls, bash command invocations, retries,
  retry sleep, rate-limit/API signals, and token/cost data where available.
- After every batch, write:
  - a result doc,
  - a retro doc,
  - concrete bashy/harness follow-ups.
- If a benchmark is too large, sample deterministically and document the sample
  rule.

## Batch Ladder

### Batch 0: ShellBench Smoke

Purpose: prove the external benchmark download/run loop and collect raw shell
runtime sanity data.

Scope:

- suite: ShellBench
- tasks: repository sample benchmarks only
- tools: no agent tools
- arms: `bashy-current`, `gnu-bash53`
- time budget: 5 minutes

Interpretation:

This is not agentic performance evidence. It is a supporting diagnostic for
raw shell/userland behavior.

### Batch 1: IBM NL2Bash Mini

Purpose: evaluate command/script generation and execution on a small standard
NL-to-Bash subset.

Scope:

- suite: IBM NL2Bash Execution Accuracy Benchmarks
- sample: first 3 tests from `bash_1`, first 2 from `bash_2`
- tools: `codex`, then `claude`, then `agy` if runtime stays under budget
- arms: `bashy-current`, `gnu-bash53`
- max runs: 10
- time budget: 30 minutes

Adapter work:

- convert each benchmark prompt into an `eval/agent-shell/tasks/...` task or a
  generated task directory;
- run the produced script through the selected shell arm;
- keep the benchmark's expected-output verifier as the source of truth.

### Batch 2: InterCode IC-Bash Mini

Purpose: run a recognized interactive Bash benchmark in a small slice.

Scope:

- suite: InterCode IC-Bash
- sample: 4 short Bash tasks selected by deterministic task ID ordering
- tools: `codex`, `claude`, `agy`
- arms: `bashy-current`, `gnu-bash53`
- time budget: 30 minutes

Adapter work:

- either run InterCode's Bash environment directly with the selected shell arm
  or extract task prompts/verifiers into the existing container harness;
- avoid changing benchmark semantics unless needed to force shell selection.

### Batch 3: Terminal-Bench Micro

Purpose: try a few full terminal-agent tasks from the current standard suite.

Scope:

- suite: Terminal-Bench
- sample: 2 low-runtime tasks with deterministic task IDs
- tools: `codex`, `claude`, `agy`
- arms: `bashy-current`, `gnu-bash53`
- time budget: 30 minutes

Adapter work:

- inspect task metadata and select tasks with small images/runtime;
- preserve task tests as verifiers;
- force all shell work through the selected shell arm.

### Batch 4: BashBench / ControlArena Mini

Purpose: test Stack Overflow-style Bash/sysadmin tasks.

Scope:

- suite: ControlArena Bash setting / BashBench
- sample: 3 main tasks, no side-task attack mode in the first pass
- tools: `codex`, `claude`, `agy`
- arms: `bashy-current`, `gnu-bash53`
- time budget: 30 minutes

Adapter work:

- use Podman backend where possible;
- run main-task success scoring first;
- defer malware/side-task control evaluation until the useful-task path is
  stable.

### Batch 5: Koala Smoke

Purpose: understand real-world shell-program compatibility/performance gaps.

Scope:

- suite: Koala
- sample: smallest runnable benchmark set after dependency inspection
- tools: no agent tools in first pass
- arms: `bashy-current`, `gnu-bash53`
- time budget: 30 minutes

Interpretation:

Koala is shell-program characterization, not agentic task success. Failures are
useful for identifying missing coreutils flags, shell semantics, or fallback
needs.

## Retro Template

Each batch retro should answer:

- What benchmark source and exact revision was used?
- What task/sample selection rule was used?
- Which arms/tools ran?
- What passed, failed, or was invalid?
- Did bashy help the agent? If yes, how?
- Did bashy slow/confuse the agent? If yes, what should be improved?
- Were failures benchmark-adapter defects, harness defects, shell/coreutils
  defects, or agent/tool behavior?
- What bashy/harness fixes should happen before the next batch?

## Current Next Step

Batch 0 (ShellBench smoke), Batch 0.5 (Koala oneliners smoke), Batch 1
(IBM NL2Bash mini), and the 2026-07-03 bashy agentic-feature slice have run.
Next: add a harder agentic slice where bashy should reduce search work, then
run the read-only IBM `bash_2` diagnostic mini, filtering out
privileged/system-mutating tasks and keeping each matrix under the 30-minute
cap.
