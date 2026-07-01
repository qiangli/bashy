# Agent Shell Evaluation Test: 20260701T181848Z-nl2bash-basic-test2-bashy-current-codex-19131

- Task: nl2bash-basic-test2
- Agent tool: codex
- Shell arm: bashy-current
- Container image: bashy-agent-shell:bashy-current
- Result: success=true, valid=true
- Wall time: 33s
- Tool calls: 10
- Bash command invocations: 7
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181848Z-nl2bash-basic-test2-bashy-current-codex-19131/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T18:18:55Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181848Z-nl2bash-basic-test2-bashy-current-codex-19131/work	/workspace	-c pwd; ls -la
2026-07-01T18:19:00Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181848Z-nl2bash-basic-test2-bashy-current-codex-19131/work	/workspace	-c printf %s\n "#!/usr/bin/env bash" "cp dir1/test.txt dir2/test.txt" > test2.sh && chmod +x test2.sh && ls -l test2.sh
2026-07-01T18:19:05Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181848Z-nl2bash-basic-test2-bashy-current-codex-19131/work	/workspace	-c ./test2.sh && test -f dir1/test.txt && test -f dir2/test.txt && ls -l dir1/test.txt dir2/test.txt
2026-07-01T18:19:13Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181848Z-nl2bash-basic-test2-bashy-current-codex-19131/work	/workspace	-c printf "%s
" "#!/usr/bin/env bash" "cp dir1/test.txt dir2/test.txt" > test2.sh && chmod +x test2.sh && sed -n l test2.sh
2026-07-01T18:19:19Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181848Z-nl2bash-basic-test2-bashy-current-codex-19131/work	/workspace	-c cat test2.sh; ./test2.sh; test -f dir1/test.txt; test -f dir2/test.txt; ls -l dir1/test.txt dir2/test.txt
2026-07-01T18:19:21Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
