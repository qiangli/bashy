# Agent Shell Evaluation Test: 20260701T151339Z-wrong-cwd-recovery-gnu-bash53-claude-43039

- Task: wrong-cwd-recovery
- Agent tool: claude
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 31s
- Tool calls: 8
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 1
- Logs: /Users/qiangli/tests/bashy-eval/runs-current/20260701T151339Z-wrong-cwd-recovery-gnu-bash53-claude-43039/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T15:13:47Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151339Z-wrong-cwd-recovery-gnu-bash53-claude-43039/work/nested/deep	/workspace/nested/deep	-c pwd; echo ---; ls -la; echo ---WORK---; ls -la /Users/qiangli/tests/bashy-eval/runs-current/20260701T151339Z-wrong-cwd-recovery-gnu-bash53-claude-43039/work
2026-07-01T15:13:54Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151339Z-wrong-cwd-recovery-gnu-bash53-claude-43039/work/nested/deep	/workspace/nested/deep	-c cd /workspace; pwd; echo ---; ls -la; echo ---FIND---; find . -type f | head -50
2026-07-01T15:13:59Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151339Z-wrong-cwd-recovery-gnu-bash53-claude-43039/work/nested/deep	/workspace/nested/deep	-c cd /workspace; echo ===README===; cat README.md; echo ===SCRIPT===; cat scripts/report.sh; echo ===DATA===; cat data/items.tsv
2026-07-01T15:14:05Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151339Z-wrong-cwd-recovery-gnu-bash53-claude-43039/work/nested/deep	/workspace/nested/deep	-c cd /workspace && bash scripts/report.sh && echo ---; cat out/report.txt
2026-07-01T15:14:10Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
