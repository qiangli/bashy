# Agent Shell Evaluation Test: 20260701T182114Z-nl2bash-basic-test3-bashy-current-claude-27190

- Task: nl2bash-basic-test3
- Agent tool: claude
- Shell arm: bashy-current
- Container image: bashy-agent-shell:bashy-current
- Result: success=true, valid=true
- Wall time: 12s
- Tool calls: 4
- Bash command invocations: 2
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 1
- Logs: /Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182114Z-nl2bash-basic-test3-bashy-current-claude-27190/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T18:21:24Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182114Z-nl2bash-basic-test3-bashy-current-claude-27190/work	/workspace	-c ./test3.sh && echo "---" && cat test.json
2026-07-01T18:21:26Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
