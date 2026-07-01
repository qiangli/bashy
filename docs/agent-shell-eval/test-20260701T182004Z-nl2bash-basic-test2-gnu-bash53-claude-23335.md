# Agent Shell Evaluation Test: 20260701T182004Z-nl2bash-basic-test2-gnu-bash53-claude-23335

- Task: nl2bash-basic-test2
- Agent tool: claude
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 24s
- Tool calls: 8
- Bash command invocations: 3
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182004Z-nl2bash-basic-test2-gnu-bash53-claude-23335/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T18:20:17Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182004Z-nl2bash-basic-test2-gnu-bash53-claude-23335/work	/workspace	-c ls -la; echo "---"; ls dir1 dir2 2>&1
2026-07-01T18:20:25Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182004Z-nl2bash-basic-test2-gnu-bash53-claude-23335/work	/workspace	-c chmod +x test2.sh && ./test2.sh && echo "exit=$?"; echo "--- dir1:"; ls dir1; echo "--- dir2:"; ls dir2
2026-07-01T18:20:28Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
