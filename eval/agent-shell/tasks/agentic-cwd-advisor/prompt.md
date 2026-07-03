You are running a shell-task benchmark under environment `__ENV_NAME__`.

Workspace: `__WORKSPACE__`
Required shell wrapper: `__EVAL_SHELL__`

Use the required shell wrapper for shell commands so the benchmark can log shell
usage. Do not invoke `bash` or `sh` directly.

Task:

Generate the sales summary by running the existing project command. The initial
working directory may not be the project root. Do not rewrite the data file or
hard-code the answer; use the checked-in script. Do not modify
`scripts/make-summary.sh`.

This is a read-only generation task, not a destructive cleanup task. Do not use
bashy dry-run or check helpers here. The intended command is:

`__EVAL_SHELL__ --shell 'cd /workspace && scripts/make-summary.sh'`

After that, the task-local check is:

`__EVAL_SHELL__ --script /workspace/.benchmark/agent-check.sh`

Acceptance:

- `reports/summary.txt` exists in the workspace root.
- The summary contains exactly `TOTAL=47`.
- `scripts/make-summary.sh` remains unchanged.
