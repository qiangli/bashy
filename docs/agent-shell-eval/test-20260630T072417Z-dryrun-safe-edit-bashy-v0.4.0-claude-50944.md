# Agent Shell Evaluation Test: 20260630T072417Z-dryrun-safe-edit-bashy-v0.4.0-claude-50944

- Task: dryrun-safe-edit
- Agent tool: claude
- Shell arm: bashy-v0.4.0
- Container image: bashy-agent-shell:bashy-v0.4.0
- Result: success=true, valid=true
- Wall time: 43s
- Tool calls: 12
- Bash command invocations: 13
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 1
- Logs: $RUN_ROOT/runs/20260630T072417Z-dryrun-safe-edit-bashy-v0.4.0-claude-50944/logs

## Verifier

```text


```

## Command Log

```text
2026-06-30T07:24:24Z	$RUN_ROOT/runs/20260630T072417Z-dryrun-safe-edit-bashy-v0.4.0-claude-50944/work	/workspace	-c ls -R; echo "---CLEANUP---"; cat scripts/cleanup.sh
2026-06-30T07:24:30Z	$RUN_ROOT/runs/20260630T072417Z-dryrun-safe-edit-bashy-v0.4.0-claude-50944/work	/workspace	-c cat README.md
2026-06-30T07:24:54Z	$RUN_ROOT/runs/20260630T072417Z-dryrun-safe-edit-bashy-v0.4.0-claude-50944/work	/workspace	-c 
chmod +x scripts/cleanup.sh
echo "=== DRY RUN ==="
./scripts/cleanup.sh --dry-run
echo "=== REAL RUN ==="
./scripts/cleanup.sh; echo "exit=$?"
echo "=== VERIFY ==="
[ -f fixtures/keep.txt ] && echo "PASS: fixtures/keep.txt exists" || echo "FAIL: fixtures/keep.txt missing"
[ -e build/tmp/generated.txt ] && echo "FAIL: generated.txt still exists" || echo "PASS: build/tmp/generated.txt removed"

2026-06-30T07:25:00Z	$REPO	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
