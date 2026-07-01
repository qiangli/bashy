# Agent Shell Evaluation Test: 20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439

- Task: nl2bash-basic-test1
- Agent tool: agy
- Shell arm: gnu-bash53
- Container image: bashy-agent-shell:gnu-bash53
- Result: success=true, valid=true
- Wall time: 49s
- Tool calls: 12
- Bash command invocations: 12
- Retries: 0
- Retry sleep: 0s
- Rate-limit/API error signals: 0
- Logs: /Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/logs

## Verifier

```text


```

## Command Log

```text
2026-07-01T18:18:09Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/work	/workspace	chmod +x test1.sh
2026-07-01T18:18:12Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/work	/workspace	echo hello
2026-07-01T18:18:15Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/work	/workspace	bash -c echo hello
2026-07-01T18:18:20Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/work	/workspace	sh -c echo hello
2026-07-01T18:18:35Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/work	/workspace	-c echo hello
2026-07-01T18:18:36Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/work	/workspace	-c chmod +x test1.sh
2026-07-01T18:18:37Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/work	/workspace	-c ./test1.sh
2026-07-01T18:18:39Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/work	/workspace	-c ls -la
2026-07-01T18:18:40Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/work	/workspace	-c ./test1.sh
2026-07-01T18:18:44Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/work	/workspace	-c rm -rf test
2026-07-01T18:18:46Z	/Users/qiangli/tests/bashy-eval/runs-nl2bash/20260701T181759Z-nl2bash-basic-test1-gnu-bash53-agy-16439/work	/workspace	-c ls -la test1.sh
2026-07-01T18:18:48Z	/Users/qiangli/projects/poc/dhnt/bashy	/workspace	/workspace/.verify.sh /workspace
```

## Notes

This run used host-agent orchestration with container-enforced task shell
execution. The compared shell arm is the container image, not the host shell.
