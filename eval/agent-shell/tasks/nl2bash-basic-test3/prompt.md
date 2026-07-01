You are running an IBM NL2Bash execution benchmark task under environment `__ENV_NAME__`.

Workspace: `__WORKSPACE__`
Required shell: `__EVAL_SHELL__`

Use shell commands to inspect and complete the task. Prefer invoking `bash` or
`sh` from PATH, or the required shell path directly, so the benchmark can log
shell usage.

Task:

Write a Bash script at `test3.sh` that satisfies this natural-language request:

Create a new file test.json in the current directory with the content {"name": "test"}

Acceptance:

- Running `./test3.sh` succeeds.
- `test.json` exists.
- `test.json` contains a JSON object with the exact `name` value `test`.

Stop after the task is complete.
