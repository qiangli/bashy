# Agent Shell Evaluation Test: 20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442

- Task: dryrun-safe-edit
- Agent tool: agy
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 3680s
- Tool calls: 16
- Bash command invocations: 16
- Retries: 1
- Retry sleep: 30s
- Rate-limit/API error signals: 1
- Logs: $RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/logs

## Verifier

```text


```

## Command Log

```text
2026-06-30T07:32:16Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	bash --version
2026-06-30T07:32:23Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	
2026-06-30T07:32:27Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	ls
2026-06-30T07:32:30Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	uname -a
2026-06-30T07:32:33Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	echo hello
2026-06-30T07:50:11Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	echo hello
2026-06-30T07:50:27Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	uname -a
2026-06-30T08:07:00Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	-c uname -a
2026-06-30T08:32:46Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	-c echo hello
2026-06-30T08:32:48Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	-c which bashy || echo 'no bashy'
2026-06-30T08:32:50Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	-c env
2026-06-30T08:32:59Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	-c ls -la /usr/local/bin /usr/bin
2026-06-30T08:33:06Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	-c bash -n scripts/cleanup.sh
2026-06-30T08:33:09Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	-c bash --version
2026-06-30T08:33:12Z	$RUN_ROOT/runs/20260630T073200Z-dryrun-safe-edit-gnu-bash53-agy-72442/work	/workspace	./scripts/cleanup.sh
2026-06-30T08:33:20Z	$REPO	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
