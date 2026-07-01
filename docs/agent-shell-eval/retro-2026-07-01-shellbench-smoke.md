# Retro - ShellBench Smoke 2026-07-01

## What Worked

- ShellBench was useful as a cheap Batch 0 probe. It exposed real shell semantics gaps before any expensive agentic benchmark run.
- The GNU control stayed honest: it used original GNU Bash 5.3 built from C source, not bashy's `bin/bash`.
- Running benchmarks through `bashy podman` worked, and the corrected `timeout setsid ...` launch made the process-group assumptions explicit.
- The smoke produced actionable fixes in fd inheritance, `$!`/nounset behavior, background-subshell `$!`, and fd export from command substitutions.

## What Should Improve

- Harness preflight must include a process-group probe before any ShellBench run:
  `setsid sh -c 'kill -0 -$$'`.
- The conductor should run a tiny reproduction before the full smoke when a benchmark uses traps, process groups, or command substitution.
- Raw rc values should be written to files from the start, not reconstructed after the run.
- Bashy needs an explicit issue/plan for signal routing in in-process command substitutions. ShellBench is blocked until that is solved or the benchmark is adapted to avoid signal coordination inside `$(...)`.

## Bashy Follow-Ups

- Decide whether command substitution should optionally fork a helper process for signal-sensitive compatibility cases.
- Alternatively, introduce internal signal routing so external children spawned from command substitution can target the command-substitution runner rather than only the top-level process.
- Add compatibility tests for:
  - `$!` under `set -u`.
  - `$!` inherited by background subshells but not waitable there.
  - `$(child >&3)` where fd 3 duplicates command-substitution stdout.
  - Signal/trap coordination where an external child signals the command-substitution parent.

## Conductor Follow-Ups

- Keep ShellBench as a diagnostic benchmark, not a bashy-vs-GNU performance score, until the command-substitution signal gap is closed.
- Move to IBM NL2Bash mini or Terminal-Bench smoke for the next agentic comparison batch; both are more representative of agent shell-script tasks and less dependent on shell-internal process-group timing.
- Continue the rule: after each batch, document result, retro, fixes, and refreshed compatibility/conformance scores.
