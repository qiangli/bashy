# Agent Shell Evaluation Test: 20260630T071800Z-wrong-cwd-recovery-gnu-bash53-claude-33921

- Task: wrong-cwd-recovery
- Agent tool: claude
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 28s
- Tool calls: 8
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 1
- Logs: $RUN_ROOT/runs/20260630T071800Z-wrong-cwd-recovery-gnu-bash53-claude-33921/logs

## Verifier

```text


```

## Command Log

```text
2026-06-30T07:18:05Z	$RUN_ROOT/runs/20260630T071800Z-wrong-cwd-recovery-gnu-bash53-claude-33921/work/nested/deep	/workspace/nested/deep	-c pwd; echo "---"; ls -la; echo "--- work root ---"; ls -la $RUN_ROOT/runs/20260630T071800Z-wrong-cwd-recovery-gnu-bash53-claude-33921/work
2026-06-30T07:18:11Z	$RUN_ROOT/runs/20260630T071800Z-wrong-cwd-recovery-gnu-bash53-claude-33921/work/nested/deep	/workspace/nested/deep	-c cd /workspace; ls -la; echo "--- find ---"; find . -maxdepth 3 -type f | head -50
2026-06-30T07:18:20Z	$RUN_ROOT/runs/20260630T071800Z-wrong-cwd-recovery-gnu-bash53-claude-33921/work/nested/deep	/workspace/nested/deep	-c cd /workspace; echo "=== README ==="; cat README.md; echo; echo "=== report.sh ==="; cat scripts/report.sh; echo; echo "=== items.tsv ==="; cat data/items.tsv
2026-06-30T07:18:24Z	$RUN_ROOT/runs/20260630T071800Z-wrong-cwd-recovery-gnu-bash53-claude-33921/work/nested/deep	/workspace/nested/deep	-c cd /workspace && bash scripts/report.sh && echo "--- result ---" && cat out/report.txt
2026-06-30T07:18:28Z	$REPO	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
