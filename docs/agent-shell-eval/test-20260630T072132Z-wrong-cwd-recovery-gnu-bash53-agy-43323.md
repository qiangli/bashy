# Agent Shell Evaluation Test: 20260630T072132Z-wrong-cwd-recovery-gnu-bash53-agy-43323

- Task: wrong-cwd-recovery
- Agent tool: agy
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 55s
- Tool calls: 5
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: $RUN_ROOT/runs/20260630T072132Z-wrong-cwd-recovery-gnu-bash53-agy-43323/logs

## Verifier

```text


```

## Command Log

```text
2026-06-30T07:22:10Z	$RUN_ROOT/runs/20260630T072132Z-wrong-cwd-recovery-gnu-bash53-agy-43323/work/nested/deep	/workspace/nested/deep	-c echo hello
2026-06-30T07:22:13Z	$RUN_ROOT/runs/20260630T072132Z-wrong-cwd-recovery-gnu-bash53-agy-43323/work/nested/deep	/workspace/nested/deep	-c pwd
2026-06-30T07:22:16Z	$RUN_ROOT/runs/20260630T072132Z-wrong-cwd-recovery-gnu-bash53-agy-43323/work/nested/deep	/workspace/nested/deep	-c cd ../.. && bash scripts/report.sh
2026-06-30T07:22:21Z	$RUN_ROOT/runs/20260630T072132Z-wrong-cwd-recovery-gnu-bash53-agy-43323/work/nested/deep	/workspace/nested/deep	-c cat ../../out/report.txt
2026-06-30T07:22:27Z	$REPO	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
