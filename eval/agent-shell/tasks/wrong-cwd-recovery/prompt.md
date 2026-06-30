You are running a shell-task benchmark under environment `__ENV_NAME__`.

Workspace: `__WORKSPACE__`
Required shell: `__EVAL_SHELL__`

Start in `__WORKSPACE__/nested/deep`. Use shell commands to complete the task.
Prefer invoking `bash` or `sh` from PATH, or the required shell path directly,
so the benchmark can log shell usage.

Task:

Run the report workflow and produce `__WORKSPACE__/out/report.txt`.

Acceptance:

- `out/report.txt` exists.
- It contains exactly `TOTAL=16`.

Do not edit files unless needed. Stop after the task is complete.

