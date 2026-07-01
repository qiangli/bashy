You are running an IBM NL2Bash execution benchmark task under environment `__ENV_NAME__`.

Workspace: `__WORKSPACE__`
Required shell: `__EVAL_SHELL__`

Use shell commands to inspect and complete the task. Prefer invoking `bash` or
`sh` from PATH, or the required shell path directly, so the benchmark can log
shell usage.

Task:

Write a Bash script at `test1.sh` that satisfies this natural-language request:

Create a new directory test in the current directory

Acceptance:

- Running `./test1.sh` succeeds.
- A directory named `test` exists in the current directory.

Stop after the task is complete.
