# Agent Shell Evaluation Test: 20260701T182028Z-nl2bash-basic-test2-gnu-bash53-agy-24449

- Task: nl2bash-basic-test2
- Agent tool: agy
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 23s
- Tool calls: 5
- Bash command invocations: 5
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182028Z-nl2bash-basic-test2-gnu-bash53-agy-24449/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T18:20:44Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182028Z-nl2bash-basic-test2-gnu-bash53-agy-24449/work	/workspace	--help
2026-07-01T18:20:45Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182028Z-nl2bash-basic-test2-gnu-bash53-agy-24449/work	/workspace	-c ls -la
2026-07-01T18:20:47Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182028Z-nl2bash-basic-test2-gnu-bash53-agy-24449/work	/workspace	-c chmod +x test2.sh && ./test2.sh
2026-07-01T18:20:48Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T182028Z-nl2bash-basic-test2-gnu-bash53-agy-24449/work	/workspace	-c ls -la dir1 dir2
2026-07-01T18:20:51Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
