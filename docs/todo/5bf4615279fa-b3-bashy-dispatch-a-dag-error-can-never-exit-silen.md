---
id: 5bf4615279fa
kind: task
title: 'B3 bashy dispatch: a dag error can never exit silently — print what cobra swallowed; verify targets run with capacity mounted'
seq: 268
status: todo
priority: p0
created: 2026-09-13T03:06:58.788584Z
sprint: 164
---

FINDING (sprint 163): 'bashy dag build' (and -n, --json, any target, any repo) exited 1 with ZERO bytes on stdout and stderr. ROOT CAUSE (debug build 2026-09-13): cobra returned 'unknown command "build" for "dag"' — bashy mounts dag.AddCapacityCommands (a 'capacity' subcommand) and once a cobra root has subcommands with no explicit Args, every positional target is treated as a subcommand lookup. The MESSAGE was lost because NewDagCmd sets SilenceErrors and internal/agentos/agentos.go does dispatchExit(dag.ExitCodeOf(cmd.Execute())) — the error is mapped to a code and never printed. Two workers lost their last ten minutes to it.

The positional-args fix is coreutils D1 (pkg/dag root Args + regression test). THIS story is bashy's half:
- internal/agentos/agentos.go case "dag": when cmd.Execute() returns a non-nil error that is NOT a *dag.Error (dag already emitted its own envelope via emitErr for those), print it: 'bashy dag: <err>' to stderr (JSON envelope when the resolved output mode is JSON — reuse weavecli's envelope helper) — a silent non-zero exit is the absence-of-evidence failure mode. Keep dag.ExitCodeOf for the status.
- Test (internal/agentos, no PTY): drive the dispatch path with args that make cobra fail before RunE (e.g. an unknown flag '--no-such-flag') and assert stderr is non-empty and contains the cobra message; and with the coreutils D1 fix pinned, assert 'dag hello' on a temp DAG.md runs (exit 0) through the same dispatch wrapper WITH AddCapacityCommands mounted — the regression the 163 workers hit.
- Bump .sibling-pins to the coreutils commit carrying D1 (published) in the same commit.
Gate: go test ./internal/agentos/... green; installed smoke by the conductor: 'bashy dag build -n' exits 0 in ycode and coreutils; 'bashy dag --no-such-flag' prints a message and exits non-zero. Files: internal/agentos/agentos.go, internal/agentos/dag_dispatch_test.go (new), .sibling-pins. Depends on: coreutils D1 pushed.
