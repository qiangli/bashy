# Agent Shell Evaluation Test: 20260701T181734Z-nl2bash-basic-test1-bashy-current-agy-15117

- Task: nl2bash-basic-test1
- Agent tool: agy
- Shell arm: bashy-current
- Container image: bashy-agent-shell:bashy-current
- Result: success=true, valid=true
- Wall time: 18s
- Tool calls: 4
- Bash command invocations: 4
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181734Z-nl2bash-basic-test1-bashy-current-agy-15117/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T18:17:46Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181734Z-nl2bash-basic-test1-bashy-current-agy-15117/work	/workspace	chmod +x test1.sh
2026-07-01T18:17:47Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181734Z-nl2bash-basic-test1-bashy-current-agy-15117/work	/workspace	./test1.sh
2026-07-01T18:17:50Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181734Z-nl2bash-basic-test1-bashy-current-agy-15117/work	/workspace	ls -la
2026-07-01T18:17:52Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
