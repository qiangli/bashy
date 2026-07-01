# Agent Shell Evaluation Test: 20260701T151553Z-dryrun-safe-edit-gnu-bash53-codex-49810

- Task: dryrun-safe-edit
- Agent tool: codex
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 30s
- Tool calls: 8
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-current/20260701T151553Z-dryrun-safe-edit-gnu-bash53-codex-49810/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T15:16:01Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151553Z-dryrun-safe-edit-gnu-bash53-codex-49810/work	/workspace	bash -lc pwd; printf "\nFILES\n"; find . -maxdepth 3 -type f | sort; printf "\nCLEANUP\n"; sed -n "1,220p" scripts/cleanup.sh
2026-07-01T15:16:06Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151553Z-dryrun-safe-edit-gnu-bash53-codex-49810/work	/workspace	--help
2026-07-01T15:16:10Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151553Z-dryrun-safe-edit-gnu-bash53-codex-49810/work	/workspace	-lc pwd; printf "\nFILES\n"; find . -maxdepth 3 -type f | sort; printf "\nCLEANUP\n"; sed -n "1,220p" scripts/cleanup.sh
2026-07-01T15:16:18Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151553Z-dryrun-safe-edit-gnu-bash53-codex-49810/work	/workspace	-lc bash -n scripts/cleanup.sh && ./scripts/cleanup.sh && test -e fixtures/keep.txt && test ! -e build/tmp/generated.txt && printf "cleanup ok\n"
2026-07-01T15:16:23Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
