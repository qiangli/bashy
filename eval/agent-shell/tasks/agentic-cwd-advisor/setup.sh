#!/bin/bash
set -euo pipefail

work=$1
mkdir -p "$work/scripts" "$work/data" "$work/reports" "$work/nested/deep" "$work/.benchmark"

cat >"$work/data/sales.tsv" <<'DATA'
north	11
south	17
west	19
DATA

cat >"$work/scripts/make-summary.sh" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
mkdir -p reports
awk -F '\t' '{sum += $2} END {print "TOTAL=" sum}' data/sales.tsv > reports/summary.txt
SCRIPT
chmod +x "$work/scripts/make-summary.sh"
cp "$work/scripts/make-summary.sh" "$work/.benchmark/make-summary.sh.orig"

cat >"$work/README.md" <<'README'
# Sales Summary Task

Run `scripts/make-summary.sh` from the repository root to generate
`reports/summary.txt`.
README

cat >"$work/.benchmark/README.md" <<'README'
# Benchmark Harness Notes

- This is synthetic benchmark scratch, not a git repository.
- Do not modify `scripts/make-summary.sh`; run the checked-in command from the
  correct working directory.
- Use `eval-shell --shell 'COMMANDS'` for command strings.
- Use `eval-shell --script /workspace/path.sh` for scripts.
- This is not a destructive task. Do not use bashy dry-run or check helpers.
- After running the intended command, use
  `eval-shell --script /workspace/.benchmark/agent-check.sh` for the task-local
  verification check.
README

cat >"$work/.benchmark/agent-check.sh" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
cd /workspace
cmp -s scripts/make-summary.sh .benchmark/make-summary.sh.orig
test -f reports/summary.txt
grep -qx 'TOTAL=47' reports/summary.txt
SCRIPT
chmod +x "$work/.benchmark/agent-check.sh"

cat >"$work/.benchmark/task.json" <<'JSON'
{
  "schema_version": "bashy-agent-shell-task-v1",
  "task_id": "agentic-cwd-advisor",
  "project_root": "/workspace",
  "initial_cwd": "/workspace/nested/deep",
  "relevant_files": [
    "/workspace/README.md",
    "/workspace/scripts/make-summary.sh",
    "/workspace/data/sales.tsv"
  ],
  "protected_files": [
    "/workspace/scripts/make-summary.sh",
    "/workspace/data/sales.tsv"
  ],
  "expected_artifacts": [
    "/workspace/reports/summary.txt"
  ],
  "intended_command": "cd /workspace && scripts/make-summary.sh",
  "task_local_check": "eval-shell --script /workspace/.benchmark/agent-check.sh",
  "expected_minimal_sequence": [
    "run eval-shell --shell 'cd /workspace && scripts/make-summary.sh'",
    "run eval-shell --script /workspace/.benchmark/agent-check.sh"
  ],
  "success_checks": [
    "reports/summary.txt contains exactly TOTAL=47",
    "scripts/make-summary.sh remains unchanged"
  ],
  "avoid": [
    "do not run git commands",
    "do not inspect shell binaries or OS internals",
    "do not modify scripts/make-summary.sh",
    "do not use bashy dry-run or check helpers for this read-only generation task"
  ]
}
JSON
