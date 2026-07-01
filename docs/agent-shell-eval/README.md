# Agent Shell Evaluation

This folder tracks the evaluation campaign comparing agent performance under
the current `bashy` AgentOS build versus `GNU Bash 5.3`.

Historical result files keep their original labels, including the 2026-06-30
`bashy v0.4.0` pilot. New product-comparison runs should use the live
`bashy-current` arm and record the exact `bashy --version` output in the run
summary.

- Plan: [`../plan-agent-shell-evaluation-sprint.md`](../plan-agent-shell-evaluation-sprint.md)
- Container harness requirement: [`container-harness.md`](container-harness.md)
- Metrics: [`metrics.md`](metrics.md)
- Run log: [`run-log.md`](run-log.md)
- Current baseline: [`current-baseline.md`](current-baseline.md)
- Latest container preflight: [`container-preflight-2026-06-29.md`](container-preflight-2026-06-29.md)
- Latest results: [`results-2026-07-01-current-baseline.md`](results-2026-07-01-current-baseline.md)
- Retros: files named `retro-YYYY-MM-DD*.md`

Runnable harness files live under `eval/agent-shell/`; this folder is the
durable human-facing record.
