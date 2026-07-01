# Agent Shell Evaluation Test: 20260701T151706Z-dryrun-safe-edit-gnu-bash53-claude-53605

- Task: dryrun-safe-edit
- Agent tool: claude
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 56s
- Tool calls: 14
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-current/20260701T151706Z-dryrun-safe-edit-gnu-bash53-claude-53605/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T15:17:21Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151706Z-dryrun-safe-edit-gnu-bash53-claude-53605/work	/workspace	-c ls -R; echo "---CLEANUP---"; cat scripts/cleanup.sh
2026-07-01T15:17:26Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151706Z-dryrun-safe-edit-gnu-bash53-claude-53605/work	/workspace	-c cat README.md; echo "---"; ls scripts
2026-07-01T15:17:50Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151706Z-dryrun-safe-edit-gnu-bash53-claude-53605/work	/workspace	-c chmod +x scripts/cleanup.sh; echo "=== DRY RUN ==="; DRYRUN=1 ./scripts/cleanup.sh; echo "=== files before ==="; ls fixtures build/tmp
2026-07-01T15:17:55Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151706Z-dryrun-safe-edit-gnu-bash53-claude-53605/work	/workspace	-c ./scripts/cleanup.sh; echo "exit=$?"; echo "=== after ==="; ls -la fixtures; echo "---"; ls -la build/tmp; echo "=== checks ==="; test -f fixtures/keep.txt && echo "keep.txt: PRESERVED" || echo "keep.txt: MISSING"; test -f build/tmp/generated.txt && echo "generated.txt: STILL PRESENT" || echo "generated.txt: REMOVED"
2026-07-01T15:18:02Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
