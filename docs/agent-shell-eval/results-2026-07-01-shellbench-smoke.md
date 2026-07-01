# ShellBench Smoke Result - 2026-07-01

## Scope

- Campaign: standard benchmark Batch 0 from `standard-benchmark-campaign.md`.
- Benchmark source: ShellBench at `~/tests/bashy-standard-benchmarks/shellbench`, commit `4b04d18ecd170948fcbb52a130c708fd3e08e1af`.
- Container harness: `bashy podman`.
- Bashy image: `bashy-agent-shell:bashy-current`, rebuilt from `5.3.0(1)-bashy-eval-shellbench3`.
- GNU control image: original GNU Bash `5.3.0(2)-release`, built from the C source in `external/bash-5.3`.
- Command launch correction: ShellBench must run as its own process-group leader, so the final command used `timeout 30 setsid /bench/shellbench ...`.

Raw files:

- `results/standard-benchmarks/shellbench-20260701-fixed/gnu-bash53-setsid.txt`
- `results/standard-benchmarks/shellbench-20260701-fixed/gnu-bash53-setsid.rc`
- `results/standard-benchmarks/shellbench-20260701-fixed/bashy-current-setsid.txt`
- `results/standard-benchmarks/shellbench-20260701-fixed/bashy-current-setsid.rc`
- Earlier diagnostics are retained in `results/standard-benchmarks/shellbench-20260701/`, `shellbench-20260701-fd3/`, and `shellbench-20260701-fixed/`.

## Result

GNU Bash completed the smoke:

| Sample | GNU Bash 5.3 |
| --- | ---: |
| `count.sh: posix` | 766,803/s |
| `count.sh: typeset -i` | 732,262/s |
| `count.sh: increment` | 955,145/s |
| `output.sh: echo` | 586,811/s |
| `output.sh: printf` | 560,357/s |
| `output.sh: print` | expected error, `print: command not found` |

Bashy did not complete the smoke. It timed out after 30 seconds on `count.sh: posix` with rc `124`.

## Bugs Found And Fixed During Batch

Two bashy/sh compatibility bugs were fixed during the smoke:

1. Recursive bashy fd inheritance missed open fds unless `BASHY_INHERITED_FDS` was already set. `internal/cli` now discovers open inherited fds at startup on Unix.
2. `../sh` did not export write-only fd table entries to external children. `execExtraFiles()` now includes write-only `*os.File` entries.
3. `../sh` treated `$!` as unset under `set -u` even after a background job existed.
4. `../sh` did not let background subshells expand the caller's `$!`; it now inherits the last-background value without making the parent job waitable.
5. `../sh` could not export inherited fds backed by in-memory command-substitution writers. It now bridges non-file fd writers through an `os.Pipe` for external children.

## Remaining Blocker

ShellBench runs the timed benchmark inside command substitution and uses an external child process's `PPID` plus `kill -HUP "$MAIN_PID"` / `kill -HUP "-$$"` to coordinate traps. GNU Bash has a real forked command-substitution process. Bashy currently runs command substitution in-process as a goroutine, so external children see the top-level bashy OS process as their parent rather than a distinct command-substitution process. The signal therefore does not wake the command-substitution runner ShellBench expects, and the benchmark times out.

This is a real compatibility gap for signal-heavy command substitutions, not a ShellBench-only artifact. It should be tracked as a conformance issue before using ShellBench as a performance comparator.

## Compatibility / Conformance Scores

Compatibility / conformance after the final patch:

- GNU Bash 5.3 compatibility fixtures: `86 passed, 0 failed, 0 skipped, 0 timed out`.
- Yash POSIX Alpine panel: bashy `1762 OK / 64 ERROR`, `96% pass (of 1826)`.
- Yash POSIX Debian panel: bashy `1776 OK / 62 ERROR`, `96% pass (of 1838)`.
