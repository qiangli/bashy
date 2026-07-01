# Agent Shell Evaluation Test: 20260701T151802Z-dryrun-safe-edit-bashy-current-agy-56584

- Task: dryrun-safe-edit
- Agent tool: agy
- Shell arm: bashy-current
- Container image: bashy-agent-shell:bashy-current
- Result: success=true, valid=true
- Wall time: 27s
- Tool calls: 6
- Bash command invocations: 6
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-current/20260701T151802Z-dryrun-safe-edit-bashy-current-agy-56584/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T15:18:17Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151802Z-dryrun-safe-edit-bashy-current-agy-56584/work	/workspace	--help
2026-07-01T15:18:18Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151802Z-dryrun-safe-edit-bashy-current-agy-56584/work	/workspace	--dryrun scripts/cleanup.sh
2026-07-01T15:18:22Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151802Z-dryrun-safe-edit-bashy-current-agy-56584/work	/workspace	--dryrun scripts/cleanup.sh
2026-07-01T15:18:23Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151802Z-dryrun-safe-edit-bashy-current-agy-56584/work	/workspace	./scripts/cleanup.sh
2026-07-01T15:18:27Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151802Z-dryrun-safe-edit-bashy-current-agy-56584/work	/workspace	-c ls -la fixtures/keep.txt build/tmp
2026-07-01T15:18:28Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
