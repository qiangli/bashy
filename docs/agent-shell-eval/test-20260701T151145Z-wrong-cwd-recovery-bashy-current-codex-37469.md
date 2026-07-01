# Agent Shell Evaluation Test: 20260701T151145Z-wrong-cwd-recovery-bashy-current-codex-37469

- Task: wrong-cwd-recovery
- Agent tool: codex
- Shell arm: bashy-current
- Container image: bashy-agent-shell:bashy-current
- Result: success=true, valid=true
- Wall time: 39s
- Tool calls: 14
- Bash command invocations: 8
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-current/20260701T151145Z-wrong-cwd-recovery-bashy-current-codex-37469/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T15:11:52Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151145Z-wrong-cwd-recovery-bashy-current-codex-37469/work/nested/deep	/workspace/nested/deep	pwd; find ../.. -maxdepth 3 -type f | sort | sed "s#^#FILE #"
2026-07-01T15:11:57Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151145Z-wrong-cwd-recovery-bashy-current-codex-37469/work/nested/deep	/workspace/nested/deep	bash -lc pwd; find ../.. -maxdepth 3 -type f | sort | sed "s#^#FILE #"
2026-07-01T15:12:02Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151145Z-wrong-cwd-recovery-bashy-current-codex-37469/work/nested/deep	/workspace/nested/deep	--help
2026-07-01T15:12:07Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151145Z-wrong-cwd-recovery-bashy-current-codex-37469/work/nested/deep	/workspace/nested/deep	-lc pwd; find ../.. -maxdepth 3 -type f | sort | sed "s#^#FILE #"
2026-07-01T15:12:15Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151145Z-wrong-cwd-recovery-bashy-current-codex-37469/work/nested/deep	/workspace/nested/deep	-lc sed -n "1,200p" ../../README.md
2026-07-01T15:12:15Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151145Z-wrong-cwd-recovery-bashy-current-codex-37469/work/nested/deep	/workspace/nested/deep	-lc sed -n "1,200p" ../../scripts/report.sh
2026-07-01T15:12:20Z	/Users/qiangli/tests/bashy-eval/runs-current/20260701T151145Z-wrong-cwd-recovery-bashy-current-codex-37469/work/nested/deep	/workspace/nested/deep	-lc cd ../.. && scripts/report.sh && test -f out/report.txt && printf "content=" && cat out/report.txt
2026-07-01T15:12:24Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
