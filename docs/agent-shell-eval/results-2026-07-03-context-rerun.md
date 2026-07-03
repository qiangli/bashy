# Agentic Context Rerun Result - 2026-07-03

## Scope

This reran the same 2-task agentic-feature slice after adding
`bashy context --json` and tightening the benchmark prompt to discourage ad hoc
environment probing.

- Tasks: `agentic-dryrun-cleanup`, `agentic-cwd-advisor`
- Tools: `codex`, `claude`, `agy`
- Arms: `bashy-current`, `gnu-bash53`
- Bashy build under test: `GNU bash, version 5.3.0(1)-bashy-context-6b9f3a1`
- Raw JSONL: `results/agent-shell-agentic-context-20260703.jsonl`
- Workspaces/logs: `/private/tmp/bashy-eval-runs-context-20260703/`

## Result

All valid runs passed: `12/12`.

| Arm | Runs | Passes | Wall | Tool calls | Shell invocations |
| --- | ---: | ---: | ---: | ---: | ---: |
| `bashy-current` | 6 | 6 | 263s | 80 | 51 |
| `gnu-bash53` | 6 | 6 | 340s | 64 | 42 |

## Before/After

| Arm | Metric | Before | After | Delta |
| --- | --- | ---: | ---: | ---: |
| `bashy-current` | Wall time | 340s | 263s | -77s |
| `bashy-current` | Tool calls | 111 | 80 | -31 |
| `bashy-current` | Shell invocations | 82 | 51 | -31 |
| `gnu-bash53` | Wall time | 314s | 340s | +26s |
| `gnu-bash53` | Tool calls | 66 | 64 | -2 |
| `gnu-bash53` | Shell invocations | 44 | 42 | -2 |

## Discovery Signal

Before the context change, bashy-current runs had visible ad hoc environment
probing in `agy` runs:

- `agentic-dryrun-cleanup/bashy-current/agy`: 8 environment probes
- `agentic-cwd-advisor/bashy-current/agy`: 2 environment probes

After the context change:

- `bashy-current` context calls: 5 of 6 runs
- `bashy-current` environment probes: 0
- `gnu-bash53` environment probes: 1

## Interpretation

The change improved discovery. `bashy context --json` gave agents the exact
`bashy_path` (`/usr/local/bin/bashy` inside the benchmark container) and the
agentic capability map in one command, replacing expensive probes such as
`env`, `uname`, `file`, `ldd`, `bashy --help`, and binary inspection.

Bashy-current is now faster on total wall time in this slice, but it still uses
more tool calls and shell invocations than GNU Bash 5.3. The remaining gap is
workflow bundling: agents still run dry-run, real execution, and post-state
checks as separate commands. The next improvement should bundle those into a
single safe run envelope that satisfies the dry-run verifier and reports final
state.
