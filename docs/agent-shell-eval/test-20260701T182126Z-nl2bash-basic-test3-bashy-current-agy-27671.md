# Agent Shell Evaluation Test: 20260701T182126Z-nl2bash-basic-test3-bashy-current-agy-27671

- Task: nl2bash-basic-test3
- Agent tool: agy
- Shell arm: bashy-current
- Container image: bashy-agent-shell:bashy-current
- Result: success=true, valid=true
- Wall time: 21s
- Tool calls: 5
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182126Z-nl2bash-basic-test3-bashy-current-agy-27671/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T18:21:38Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182126Z-nl2bash-basic-test3-bashy-current-agy-27671/work	/workspace	chmod +x test3.sh
2026-07-01T18:21:42Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182126Z-nl2bash-basic-test3-bashy-current-agy-27671/work	/workspace	chmod +x test3.sh
2026-07-01T18:21:43Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182126Z-nl2bash-basic-test3-bashy-current-agy-27671/work	/workspace	./test3.sh
2026-07-01T18:21:46Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182126Z-nl2bash-basic-test3-bashy-current-agy-27671/work	/workspace	cat test.json
2026-07-01T18:21:48Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
