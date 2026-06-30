# Agent Shell Evaluation Test: 20260630T072258Z-dryrun-safe-edit-bashy-v0.4.0-codex-47354

- Task: dryrun-safe-edit
- Agent tool: codex
- Shell arm: bashy-v0.4.0
- Container image: bashy-agent-shell:bashy-v0.4.0
- Result: success=true, valid=true
- Wall time: 25s
- Tool calls: 4
- Bash command invocations: 3
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: $RUN_ROOT/runs/20260630T072258Z-dryrun-safe-edit-bashy-v0.4.0-codex-47354/logs

## Verifier

```text


```

## Command Log

```text
2026-06-30T07:23:04Z	$RUN_ROOT/runs/20260630T072258Z-dryrun-safe-edit-bashy-v0.4.0-codex-47354/work	/workspace	-lc pwd; printf "\nfiles:\n"; find . -maxdepth 3 -type f | sort; printf "\ncleanup:\n"; sed -n "1,200p" scripts/cleanup.sh
2026-06-30T07:23:17Z	$RUN_ROOT/runs/20260630T072258Z-dryrun-safe-edit-bashy-v0.4.0-codex-47354/work	/workspace	-lc bash -n ./scripts/cleanup.sh && ./scripts/cleanup.sh && test -e fixtures/keep.txt && test ! -e build/tmp/generated.txt && printf "cleanup acceptance passed\n"
2026-06-30T07:23:23Z	$REPO	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
