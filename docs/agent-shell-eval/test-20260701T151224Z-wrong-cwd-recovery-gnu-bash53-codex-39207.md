# Agent Shell Evaluation Test: 20260701T151224Z-wrong-cwd-recovery-gnu-bash53-codex-39207

- Task: wrong-cwd-recovery
- Agent tool: codex
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 44s
- Tool calls: 10
- Bash command invocations: 6
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-current/20260701T151224Z-wrong-cwd-recovery-gnu-bash53-codex-39207/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T15:12:39Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151224Z-wrong-cwd-recovery-gnu-bash53-codex-39207/work/nested/deep	/workspace/nested/deep	-lc pwd; find ../.. -maxdepth 3 -type f -print | sort | sed "s#^#/#"
2026-07-01T15:12:49Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151224Z-wrong-cwd-recovery-gnu-bash53-codex-39207/work/nested/deep	/workspace/nested/deep	-lc sed -n "1,200p" ../../README.md
2026-07-01T15:12:49Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151224Z-wrong-cwd-recovery-gnu-bash53-codex-39207/work/nested/deep	/workspace/nested/deep	-lc sed -n "1,200p" ../../scripts/report.sh
2026-07-01T15:12:49Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151224Z-wrong-cwd-recovery-gnu-bash53-codex-39207/work/nested/deep	/workspace/nested/deep	-lc cat ../../data/items.tsv
2026-07-01T15:12:58Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151224Z-wrong-cwd-recovery-gnu-bash53-codex-39207/work/nested/deep	/workspace/nested/deep	-lc cd ../.. && scripts/report.sh && test -f out/report.txt && cat out/report.txt
2026-07-01T15:13:08Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
