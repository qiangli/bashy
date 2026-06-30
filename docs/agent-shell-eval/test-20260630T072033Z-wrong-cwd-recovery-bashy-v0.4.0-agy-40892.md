# Agent Shell Evaluation Test: 20260630T072033Z-wrong-cwd-recovery-bashy-v0.4.0-agy-40892

- Task: wrong-cwd-recovery
- Agent tool: agy
- Shell arm: bashy-v0.4.0
- Container image: bashy-agent-shell:bashy-v0.4.0
- Result: success=true, valid=true
- Wall time: 55s
- Tool calls: 6
- Bash command invocations: 6
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: $RUN_ROOT/runs/20260630T072033Z-wrong-cwd-recovery-bashy-v0.4.0-agy-40892/logs

## Verifier

```text


```

## Command Log

```text
2026-06-30T07:20:46Z	$RUN_ROOT/runs/20260630T072033Z-wrong-cwd-recovery-bashy-v0.4.0-agy-40892/work/nested/deep	/workspace/nested/deep	-c ls -la
2026-06-30T07:21:07Z	$RUN_ROOT/runs/20260630T072033Z-wrong-cwd-recovery-bashy-v0.4.0-agy-40892/work/nested/deep	/workspace/nested/deep	-c pwd
2026-06-30T07:21:09Z	$RUN_ROOT/runs/20260630T072033Z-wrong-cwd-recovery-bashy-v0.4.0-agy-40892/work/nested/deep	/workspace/nested/deep	--help
2026-06-30T07:21:16Z	$RUN_ROOT/runs/20260630T072033Z-wrong-cwd-recovery-bashy-v0.4.0-agy-40892/work/nested/deep	/workspace/nested/deep	-c cd ../.. && ./scripts/report.sh
2026-06-30T07:21:20Z	$RUN_ROOT/runs/20260630T072033Z-wrong-cwd-recovery-bashy-v0.4.0-agy-40892/work/nested/deep	/workspace/nested/deep	-c cat ../../out/report.txt
2026-06-30T07:21:28Z	$REPO	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
