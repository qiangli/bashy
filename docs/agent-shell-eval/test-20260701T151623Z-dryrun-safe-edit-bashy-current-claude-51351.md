# Agent Shell Evaluation Test: 20260701T151623Z-dryrun-safe-edit-bashy-current-claude-51351

- Task: dryrun-safe-edit
- Agent tool: claude
- Shell arm: bashy-current
- Container image: bashy-agent-shell:bashy-current
- Result: success=true, valid=true
- Wall time: 43s
- Tool calls: 12
- Bash command invocations: 4
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-current/20260701T151623Z-dryrun-safe-edit-bashy-current-claude-51351/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T15:16:30Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151623Z-dryrun-safe-edit-bashy-current-claude-51351/work	/workspace	-c echo "=== tree ==="; ls -R; echo "=== cleanup.sh ==="; cat scripts/cleanup.sh
2026-07-01T15:16:34Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151623Z-dryrun-safe-edit-bashy-current-claude-51351/work	/workspace	-c echo "=== README ==="; cat README.md
2026-07-01T15:16:58Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151623Z-dryrun-safe-edit-bashy-current-claude-51351/work	/workspace	-c chmod +x scripts/cleanup.sh; echo "=== DRY RUN ==="; DRY_RUN=1 ./scripts/cleanup.sh; echo "=== REAL RUN ==="; ./scripts/cleanup.sh; echo "rc=$?"; echo "=== VERIFY ==="; test -f fixtures/keep.txt && echo "fixtures/keep.txt EXISTS (ok)"; test ! -e build/tmp/generated.txt && echo "build/tmp/generated.txt REMOVED (ok)"
2026-07-01T15:17:06Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
