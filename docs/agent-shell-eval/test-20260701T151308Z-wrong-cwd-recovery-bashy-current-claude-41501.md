# Agent Shell Evaluation Test: 20260701T151308Z-wrong-cwd-recovery-bashy-current-claude-41501

- Task: wrong-cwd-recovery
- Agent tool: claude
- Shell arm: bashy-current
- Container image: bashy-agent-shell:bashy-current
- Result: success=true, valid=true
- Wall time: 31s
- Tool calls: 8
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-current/20260701T151308Z-wrong-cwd-recovery-bashy-current-claude-41501/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T15:13:16Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151308Z-wrong-cwd-recovery-bashy-current-claude-41501/work/nested/deep	/workspace/nested/deep	-c pwd; echo "---"; ls -la; echo "--- work root ---"; ls -la /Users/qiangli/tests/bashy-eval/runs-current/20260701T151308Z-wrong-cwd-recovery-bashy-current-claude-41501/work
2026-07-01T15:13:21Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151308Z-wrong-cwd-recovery-bashy-current-claude-41501/work/nested/deep	/workspace/nested/deep	-c cd /workspace; ls -la; echo "--- find ---"; find . -maxdepth 3 -type f | head -50
2026-07-01T15:13:27Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151308Z-wrong-cwd-recovery-bashy-current-claude-41501/work/nested/deep	/workspace/nested/deep	-c cd /workspace; echo "=== README ==="; cat README.md; echo; echo "=== report.sh ==="; cat scripts/report.sh; echo; echo "=== items.tsv ==="; cat data/items.tsv
2026-07-01T15:13:32Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151308Z-wrong-cwd-recovery-bashy-current-claude-41501/work/nested/deep	/workspace/nested/deep	-c cd /workspace && bash scripts/report.sh && echo "--- result ---" && cat out/report.txt
2026-07-01T15:13:38Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
