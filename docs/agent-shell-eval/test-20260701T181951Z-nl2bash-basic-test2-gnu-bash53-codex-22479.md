# Agent Shell Evaluation Test: 20260701T181951Z-nl2bash-basic-test2-gnu-bash53-codex-22479

- Task: nl2bash-basic-test2
- Agent tool: codex
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 13s
- Tool calls: 4
- Bash command invocations: 3
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181951Z-nl2bash-basic-test2-gnu-bash53-codex-22479/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T18:19:56Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181951Z-nl2bash-basic-test2-gnu-bash53-codex-22479/work	/workspace	-c pwd; ls -la
2026-07-01T18:20:01Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181951Z-nl2bash-basic-test2-gnu-bash53-codex-22479/work	/workspace	-c printf "%s\n" "#!/usr/bin/env bash" "cp dir1/test.txt dir2/" > test2.sh && chmod +x test2.sh && ./test2.sh && test -f dir1/test.txt && test -f dir2/test.txt
2026-07-01T18:20:04Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
