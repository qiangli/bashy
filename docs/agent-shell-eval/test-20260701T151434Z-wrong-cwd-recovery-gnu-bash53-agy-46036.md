# Agent Shell Evaluation Test: 20260701T151434Z-wrong-cwd-recovery-gnu-bash53-agy-46036

- Task: wrong-cwd-recovery
- Agent tool: agy
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 20s
- Tool calls: 3
- Bash command invocations: 3
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-current/20260701T151434Z-wrong-cwd-recovery-gnu-bash53-agy-46036/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T15:14:46Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151434Z-wrong-cwd-recovery-gnu-bash53-agy-46036/work/nested/deep	/workspace/nested/deep	-c pwd
2026-07-01T15:14:51Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151434Z-wrong-cwd-recovery-gnu-bash53-agy-46036/work/nested/deep	/workspace/nested/deep	-c cd ../.. && bash scripts/report.sh
2026-07-01T15:14:54Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
