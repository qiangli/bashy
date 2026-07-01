# Agent Shell Evaluation Test: 20260701T182148Z-nl2bash-basic-test3-gnu-bash53-codex-29228

- Task: nl2bash-basic-test3
- Agent tool: codex
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 28s
- Tool calls: 8
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182148Z-nl2bash-basic-test3-gnu-bash53-codex-29228/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T18:21:53Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182148Z-nl2bash-basic-test3-gnu-bash53-codex-29228/work	/workspace	pwd; ls -la
2026-07-01T18:21:58Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182148Z-nl2bash-basic-test3-gnu-bash53-codex-29228/work	/workspace	bash -lc pwd; ls -la
2026-07-01T18:22:03Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182148Z-nl2bash-basic-test3-gnu-bash53-codex-29228/work	/workspace	-c pwd; ls -la
2026-07-01T18:22:11Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182148Z-nl2bash-basic-test3-gnu-bash53-codex-29228/work	/workspace	-c chmod +x ./test3.sh && ./test3.sh && test -f test.json && grep -Eq '"name"[[:space:]]*:[[:space:]]*"test"' test.json && cat test.json
2026-07-01T18:22:16Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
