# Agent Shell Evaluation Test: 20260701T181933Z-nl2bash-basic-test2-bashy-current-agy-21627

- Task: nl2bash-basic-test2
- Agent tool: agy
- Shell arm: bashy-current
- Container image: bashy-agent-shell:bashy-current
- Result: success=true, valid=true
- Wall time: 18s
- Tool calls: 5
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181933Z-nl2bash-basic-test2-bashy-current-agy-21627/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T18:19:46Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181933Z-nl2bash-basic-test2-bashy-current-agy-21627/work	/workspace	ls -la
2026-07-01T18:19:48Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181933Z-nl2bash-basic-test2-bashy-current-agy-21627/work	/workspace	chmod +x test2.sh
2026-07-01T18:19:48Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181933Z-nl2bash-basic-test2-bashy-current-agy-21627/work	/workspace	./test2.sh
2026-07-01T18:19:49Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181933Z-nl2bash-basic-test2-bashy-current-agy-21627/work	/workspace	ls -la dir1 dir2
2026-07-01T18:19:51Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
