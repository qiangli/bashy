# Agent Shell Evaluation Test: 20260630T071352Z-wrong-cwd-recovery-gnu-bash53-codex-23054

- Task: wrong-cwd-recovery
- Agent tool: codex
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 40s
- Tool calls: 14
- Bash command invocations: 8
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: $RUN_ROOT/runs/20260630T071352Z-wrong-cwd-recovery-gnu-bash53-codex-23054/logs

## Verifier

```text


```

## Command Log

```text
2026-06-30T07:13:58Z	$RUN_ROOT/runs/20260630T071352Z-wrong-cwd-recovery-gnu-bash53-codex-23054/work/nested/deep	/workspace/nested/deep	pwd; find ../.. -maxdepth 3 -type f | sort | sed "s#^#FILE #"
2026-06-30T07:14:05Z	$RUN_ROOT/runs/20260630T071352Z-wrong-cwd-recovery-gnu-bash53-codex-23054/work/nested/deep	/workspace/nested/deep	bash -lc pwd; find ../.. -maxdepth 3 -type f | sort | sed "s#^#FILE #"
2026-06-30T07:14:10Z	$RUN_ROOT/runs/20260630T071352Z-wrong-cwd-recovery-gnu-bash53-codex-23054/work/nested/deep	/workspace/nested/deep	--help
2026-06-30T07:14:15Z	$RUN_ROOT/runs/20260630T071352Z-wrong-cwd-recovery-gnu-bash53-codex-23054/work/nested/deep	/workspace/nested/deep	-lc pwd; find ../.. -maxdepth 3 -type f | sort | sed "s#^#FILE #"
2026-06-30T07:14:23Z	$RUN_ROOT/runs/20260630T071352Z-wrong-cwd-recovery-gnu-bash53-codex-23054/work/nested/deep	/workspace/nested/deep	-lc sed -n "1,160p" ../../README.md
2026-06-30T07:14:23Z	$RUN_ROOT/runs/20260630T071352Z-wrong-cwd-recovery-gnu-bash53-codex-23054/work/nested/deep	/workspace/nested/deep	-lc sed -n "1,200p" ../../scripts/report.sh
2026-06-30T07:14:28Z	$RUN_ROOT/runs/20260630T071352Z-wrong-cwd-recovery-gnu-bash53-codex-23054/work/nested/deep	/workspace/nested/deep	-lc cd ../.. && scripts/report.sh && test -f out/report.txt && cat out/report.txt
2026-06-30T07:14:32Z	$REPO	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
