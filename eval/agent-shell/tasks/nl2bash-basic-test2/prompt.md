You are running an IBM NL2Bash execution benchmark task under environment `__ENV_NAME__`.

Workspace: `__WORKSPACE__`
Required shell: `__EVAL_SHELL__`

Use shell commands to inspect and complete the task. Prefer invoking `bash` or
`sh` from PATH, or the required shell path directly, so the benchmark can log
shell usage.

Task:

Write a Bash script at `test2.sh` that satisfies this natural-language request:

Copy file test.txt from directory dir1 to dir2 in the current directory

Acceptance:

- Running `./test2.sh` succeeds.
- `dir1/test.txt` still exists.
- `dir2/test.txt` exists.

Stop after the task is complete.
