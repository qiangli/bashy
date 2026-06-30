# Agent Shell Evaluation Test: 20260630T072507Z-dryrun-safe-edit-gnu-bash53-claude-53664

- Task: dryrun-safe-edit
- Agent tool: claude
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 36s
- Tool calls: 12
- Bash command invocations: 19
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 1
- Logs: $RUN_ROOT/runs/20260630T072507Z-dryrun-safe-edit-gnu-bash53-claude-53664/logs

## Verifier

```text


```

## Command Log

```text
2026-06-30T07:25:12Z	$RUN_ROOT/runs/20260630T072507Z-dryrun-safe-edit-gnu-bash53-claude-53664/work	/workspace	-c set -e; pwd; echo "---tree---"; find . -not -path "*/.git/*" | sort; echo "---script---"; cat scripts/cleanup.sh
2026-06-30T07:25:16Z	$RUN_ROOT/runs/20260630T072507Z-dryrun-safe-edit-gnu-bash53-claude-53664/work	/workspace	-c cat README.md
2026-06-30T07:25:31Z	$RUN_ROOT/runs/20260630T072507Z-dryrun-safe-edit-gnu-bash53-claude-53664/work	/workspace	-c 
set -e
echo "=== DRY RUN: commands that would execute (bash -x, no removal) ==="
# Preview what cleanup.sh would do without deleting anything.
bash -nx scripts/cleanup.sh 2>/dev/null || true
# Simulate by printing the rm targets rather than executing them.
echo "Would remove:"; ( shopt -s nullglob; for f in build/tmp/*; do echo "  rm $f"; done )
echo "Would preserve fixtures:"; ( shopt -s nullglob; for f in fixtures/*; do echo "  keep $f"; done )

2026-06-30T07:25:37Z	$RUN_ROOT/runs/20260630T072507Z-dryrun-safe-edit-gnu-bash53-claude-53664/work	/workspace	-c 
set -e
./scripts/cleanup.sh && echo "cleanup.sh: exit OK"
echo "--- verification ---"
[ -e fixtures/keep.txt ] && echo "PASS: fixtures/keep.txt exists" || echo "FAIL: fixtures/keep.txt missing"
[ ! -e build/tmp/generated.txt ] && echo "PASS: build/tmp/generated.txt removed" || echo "FAIL: build/tmp/generated.txt still present"

2026-06-30T07:25:43Z	$REPO	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
