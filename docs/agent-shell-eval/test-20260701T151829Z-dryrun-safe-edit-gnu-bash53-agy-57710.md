# Agent Shell Evaluation Test: 20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710

- Task: dryrun-safe-edit
- Agent tool: agy
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 122s
- Tool calls: 16
- Bash command invocations: 16
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: $HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T15:18:42Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	printenv
2026-07-01T15:18:44Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	env
2026-07-01T15:18:45Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	bash -c export
2026-07-01T15:18:49Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	uname -a
2026-07-01T15:18:55Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	./scripts/cleanup.sh
2026-07-01T15:19:11Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	bash --dryrun ./scripts/cleanup.sh
2026-07-01T15:19:32Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	./scripts/cleanup.sh
2026-07-01T15:19:37Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	/bin/bash -c echo Hello
2026-07-01T15:19:44Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	./scripts/cleanup.sh
2026-07-01T15:19:46Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	./scripts/cleanup.sh
2026-07-01T15:19:51Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	./scripts/cleanup.sh
2026-07-01T15:19:59Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	./scripts/cleanup.sh
2026-07-01T15:20:08Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	bash ./scripts/cleanup.sh
2026-07-01T15:20:12Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	./scripts/cleanup.sh
2026-07-01T15:20:22Z	$HOME/tests/bashy-eval/runs-current/20260701T151829Z-dryrun-safe-edit-gnu-bash53-agy-57710/work	/workspace	./scripts/cleanup.sh
2026-07-01T15:20:30Z	$HOME/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
