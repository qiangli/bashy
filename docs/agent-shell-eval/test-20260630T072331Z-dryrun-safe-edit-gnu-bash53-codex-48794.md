# Agent Shell Evaluation Test: 20260630T072331Z-dryrun-safe-edit-gnu-bash53-codex-48794

- Task: dryrun-safe-edit
- Agent tool: codex
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 40s
- Tool calls: 10
- Bash command invocations: 6
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: $RUN_ROOT/runs/20260630T072331Z-dryrun-safe-edit-gnu-bash53-codex-48794/logs

## Verifier

```text


```

## Command Log

```text
2026-06-30T07:23:36Z	$RUN_ROOT/runs/20260630T072331Z-dryrun-safe-edit-gnu-bash53-codex-48794/work	/workspace	bash -lc pwd; printf "\nfiles:\n"; find . -maxdepth 3 -type f | sort; printf "\ncleanup:\n"; sed -n "1,200p" scripts/cleanup.sh
2026-06-30T07:23:43Z	$RUN_ROOT/runs/20260630T072331Z-dryrun-safe-edit-gnu-bash53-codex-48794/work	/workspace	--help
2026-06-30T07:23:47Z	$RUN_ROOT/runs/20260630T072331Z-dryrun-safe-edit-gnu-bash53-codex-48794/work	/workspace	-lc pwd; printf "\nfiles:\n"; find . -maxdepth 3 -type f | sort; printf "\ncleanup:\n"; sed -n "1,200p" scripts/cleanup.sh
2026-06-30T07:23:55Z	$RUN_ROOT/runs/20260630T072331Z-dryrun-safe-edit-gnu-bash53-codex-48794/work	/workspace	-lc sed -n "1,200p" README.md
2026-06-30T07:24:06Z	$RUN_ROOT/runs/20260630T072331Z-dryrun-safe-edit-gnu-bash53-codex-48794/work	/workspace	-lc bash -n scripts/cleanup.sh && ./scripts/cleanup.sh && test -e fixtures/keep.txt && test ! -e build/tmp/generated.txt && printf "cleanup acceptance checks passed\n"
2026-06-30T07:24:11Z	$REPO	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
