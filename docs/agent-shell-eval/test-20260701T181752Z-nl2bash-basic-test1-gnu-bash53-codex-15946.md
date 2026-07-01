# Agent Shell Evaluation Test: 20260701T181752Z-nl2bash-basic-test1-gnu-bash53-codex-15946

- Task: nl2bash-basic-test1
- Agent tool: codex
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 7s
- Tool calls: 2
- Bash command invocations: 2
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181752Z-nl2bash-basic-test1-gnu-bash53-codex-15946/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T18:17:57Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181752Z-nl2bash-basic-test1-gnu-bash53-codex-15946/work	/workspace	-lc printf %s\\n "#!/usr/bin/env bash" "mkdir -p test" > test1.sh && chmod +x test1.sh && ./test1.sh && test -d test
2026-07-01T18:17:59Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
