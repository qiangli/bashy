# Agent Shell Evaluation Current Baseline

Date: 2026-07-01

This baseline resumes the agent-shell evaluation after the bashy development
work that followed the 2026-06-30 `bashy v0.4.0` pilot. New runs compare the
current `bashy` development build against GNU Bash 5.3, not the old v0.4.0
image.

## Active Arms

- `bashy-current`: current `cmd/bashy` build, installed in the evaluation
  container as `/usr/local/bin/bashy`; `/bin/bash` and `/bin/sh` point to it.
- `gnu-bash53`: real GNU Bash 5.3 control image.

The harness image tags are:

```text
bashy-agent-shell:bashy-current
bashy-agent-shell:gnu-bash53
```

Every preflight must record:

- `git rev-parse --short=12 HEAD`
- host `bin/bashy --version`
- container `bashy-agent-shell:bashy-current --version`
- container GNU Bash first version line

## Scope

Approved tools for this resumed sprint:

- `codex`
- `claude`
- `agy`

Excluded until explicit approval with a cost estimate:

- `opencode`
- `aider`

## First Resumed Run

Use a small matrix to validate the refreshed baseline before scaling:

- tasks: `wrong-cwd-recovery`, `dryrun-safe-edit`
- arms: `bashy-current`, `gnu-bash53`
- tools: `codex`, `claude`, `agy`
- total runs: 12

Estimated wall time after images are warm:

- typical: 20-60 minutes
- worst case with subscription/API retry: 90 minutes

Budget exposure:

- no per-run paid budget concern for `codex`, `claude`, or `agy`
- rate limits/API timeouts are still expected operational failures and must be
  recorded as retries, retry sleep, and API/rate-limit error signals

Status: completed on 2026-07-01. See
[`results-2026-07-01-current-baseline.md`](results-2026-07-01-current-baseline.md)
and
[`retro-2026-07-01-current-baseline.md`](retro-2026-07-01-current-baseline.md).

## Metrics

Record the normal JSONL row plus the human-facing summary:

- wall time
- native token usage where the tool exposes it
- tool calls
- bash command invocations
- pass/fail total and ratio
- retry count and retry sleep
- API/rate-limit error signals
- invalid/contaminated runs
- observed bashy feature discovery, successful or failed

## Retro Rule

Each evaluation batch must produce a short retro. The retro should separate:

- harness defects
- conductor defects
- bashy product shortcomings
- agent/tool-specific behavior

Any concrete bashy shortcoming should become either an immediate fix or a
documented follow-up. The evaluation campaign is not only a benchmark; it is a
feedback loop for making bashy better for agentic tools.
