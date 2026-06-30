# Agent Shell Evaluation Test: 20260630T071253Z-wrong-cwd-recovery-bashy-v0.4.0-codex-20371

- Task: wrong-cwd-recovery
- Agent tool: codex
- Shell arm: bashy-v0.4.0
- Container image: bashy-agent-shell:bashy-v0.4.0
- Result: success=true, valid=true
- Wall time: 44s
- Tool calls: 18
- Bash command invocations: 10
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: $RUN_ROOT/runs/20260630T071253Z-wrong-cwd-recovery-bashy-v0.4.0-codex-20371/logs

## Verifier

```text


```

## Command Log

```text
2026-06-30T07:12:58Z	$RUN_ROOT/runs/20260630T071253Z-wrong-cwd-recovery-bashy-v0.4.0-codex-20371/work/nested/deep	/workspace/nested/deep	pwd; find .. -maxdepth 4 -type f | sort | sed -n "1,120p"
2026-06-30T07:13:03Z	$RUN_ROOT/runs/20260630T071253Z-wrong-cwd-recovery-bashy-v0.4.0-codex-20371/work/nested/deep	/workspace/nested/deep	bash -lc pwd; find .. -maxdepth 4 -type f | sort | sed -n "1,120p"
2026-06-30T07:13:11Z	$RUN_ROOT/runs/20260630T071253Z-wrong-cwd-recovery-bashy-v0.4.0-codex-20371/work/nested/deep	/workspace/nested/deep	--help
2026-06-30T07:13:16Z	$RUN_ROOT/runs/20260630T071253Z-wrong-cwd-recovery-bashy-v0.4.0-codex-20371/work/nested/deep	/workspace/nested/deep	-lc pwd; find .. -maxdepth 4 -type f | sort | sed -n "1,120p"
2026-06-30T07:13:20Z	$RUN_ROOT/runs/20260630T071253Z-wrong-cwd-recovery-bashy-v0.4.0-codex-20371/work/nested/deep	/workspace/nested/deep	-lc cd /workspace && find . -maxdepth 4 -type f | sort | sed -n "1,200p"
2026-06-30T07:13:28Z	$RUN_ROOT/runs/20260630T071253Z-wrong-cwd-recovery-bashy-v0.4.0-codex-20371/work/nested/deep	/workspace/nested/deep	-lc cd /workspace && sed -n "1,80p" data/items.tsv
2026-06-30T07:13:28Z	$RUN_ROOT/runs/20260630T071253Z-wrong-cwd-recovery-bashy-v0.4.0-codex-20371/work/nested/deep	/workspace/nested/deep	-lc cd /workspace && sed -n "1,200p" scripts/report.sh
2026-06-30T07:13:28Z	$RUN_ROOT/runs/20260630T071253Z-wrong-cwd-recovery-bashy-v0.4.0-codex-20371/work/nested/deep	/workspace/nested/deep	-lc cd /workspace && sed -n "1,160p" README.md
2026-06-30T07:13:33Z	$RUN_ROOT/runs/20260630T071253Z-wrong-cwd-recovery-bashy-v0.4.0-codex-20371/work/nested/deep	/workspace/nested/deep	-lc cd /workspace && scripts/report.sh && test -f out/report.txt && cat out/report.txt
2026-06-30T07:13:37Z	$REPO	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
