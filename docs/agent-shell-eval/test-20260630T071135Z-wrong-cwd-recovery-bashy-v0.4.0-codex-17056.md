# Agent Shell Evaluation Test: 20260630T071135Z-wrong-cwd-recovery-bashy-v0.4.0-codex-17056

- Task: wrong-cwd-recovery
- Agent tool: codex
- Shell arm: bashy-v0.4.0
- Container image: bashy-agent-shell:bashy-v0.4.0
- Result: success=false, valid=review
- Wall time: 25s
- Tool calls: 8
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: $RUN_ROOT/runs/20260630T071135Z-wrong-cwd-recovery-bashy-v0.4.0-codex-17056/logs

## Verifier

```text

WARN container: host resource probe failed; using fallback VM defaults mem_err="sysctl hw.memsize: exec: \"sysctl\": executable file not found in $PATH" disk_err=<nil>
/bin/bash: /bin/bash: cannot execute binary file
```

## Command Log

```text
2026-06-30T07:11:41Z	$RUN_ROOT/runs/20260630T071135Z-wrong-cwd-recovery-bashy-v0.4.0-codex-17056/work/nested/deep	/workspace/nested/deep	-lc pwd; find ../.. -maxdepth 3 -type f -o -type d | sort | sed -n "1,120p"
2026-06-30T07:11:50Z	$RUN_ROOT/runs/20260630T071135Z-wrong-cwd-recovery-bashy-v0.4.0-codex-17056/work/nested/deep	/workspace/nested/deep	-lc sed -n "1,200p" ../../README.md
2026-06-30T07:11:50Z	$RUN_ROOT/runs/20260630T071135Z-wrong-cwd-recovery-bashy-v0.4.0-codex-17056/work/nested/deep	/workspace/nested/deep	-lc sed -n "1,200p" ../../scripts/report.sh; printf "\n-- items --\n"; sed -n "1,80p" ../../data/items.tsv
2026-06-30T07:11:57Z	$RUN_ROOT/runs/20260630T071135Z-wrong-cwd-recovery-bashy-v0.4.0-codex-17056/work/nested/deep	/workspace/nested/deep	-lc cd ../.. && bash scripts/report.sh && test -f out/report.txt && cat out/report.txt
2026-06-30T07:12:00Z	$REPO	/workspace	/bin/bash /workspace/.verify.sh
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
