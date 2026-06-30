# Agent Shell Evaluation Test: 20260630T071723Z-wrong-cwd-recovery-bashy-v0.4.0-claude-32509

- Task: wrong-cwd-recovery
- Agent tool: claude
- Shell arm: bashy-v0.4.0
- Container image: bashy-agent-shell:bashy-v0.4.0
- Result: success=true, valid=true
- Wall time: 23s
- Tool calls: 8
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 1
- Logs: $RUN_ROOT/runs/20260630T071723Z-wrong-cwd-recovery-bashy-v0.4.0-claude-32509/logs

## Verifier

```text


```

## Command Log

```text
2026-06-30T07:17:29Z	$RUN_ROOT/runs/20260630T071723Z-wrong-cwd-recovery-bashy-v0.4.0-claude-32509/work/nested/deep	/workspace/nested/deep	-c pwd; echo "---"; ls -la; echo "--- work root ---"; ls -la $RUN_ROOT/runs/20260630T071723Z-wrong-cwd-recovery-bashy-v0.4.0-claude-32509/work
2026-06-30T07:17:34Z	$RUN_ROOT/runs/20260630T071723Z-wrong-cwd-recovery-bashy-v0.4.0-claude-32509/work/nested/deep	/workspace/nested/deep	-c cd /workspace; pwd; echo "---"; ls -la; echo "--- find ---"; find /workspace -maxdepth 3 -not -path "*/.git/*" | sort
2026-06-30T07:17:38Z	$RUN_ROOT/runs/20260630T071723Z-wrong-cwd-recovery-bashy-v0.4.0-claude-32509/work/nested/deep	/workspace/nested/deep	-c echo "=== README ==="; cat /workspace/README.md; echo; echo "=== report.sh ==="; cat /workspace/scripts/report.sh; echo; echo "=== items.tsv ==="; cat /workspace/data/items.tsv
2026-06-30T07:17:42Z	$RUN_ROOT/runs/20260630T071723Z-wrong-cwd-recovery-bashy-v0.4.0-claude-32509/work/nested/deep	/workspace/nested/deep	-c cd /workspace && bash scripts/report.sh && echo "--- result ---" && cat out/report.txt
2026-06-30T07:17:46Z	$REPO	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
