---
id: 42f79daac406
kind: bug
title: Adapter cold build fork-bombs under BASHY_AGENTIC=1
seq: 3298
status: todo
priority: p0
labels:
    - adapters
created: 2026-11-22T12:52:13.231974Z
sprint: 692
sprint_id: 58370405-625f-5802-b886-32f3bd57dece
sprint_title: '677 closure: green integration (go test), WSL keep-alive, symbol strip, dependency bumps, retire switch'
---

Symptom: on a fresh bashy checkout (bashy main 8fcceee7), `tools/installbashy` hung at its `judge --help` dispatch probe and spawned about 2400 processes before it was killed. Nothing was installed.

Reproduced on the dev box with an empty cache (BASHY_ADAPTER_CACHE pointing at an empty directory):
- `BASHY_AGENTIC=0 bin/bashy judge --help`: builds the yoke/judge adapter, prints help, exits 0.
- `BASHY_AGENTIC=1 bin/bashy judge --help`: unbounded recursion, more than 128 processes within 5 s.
- `toolchain/go/go.bsh version` run directly with BASHY_ADAPTER_EXEC=1 and an empty cache: fine.

Chain observed: judge.bsh `run_go build` -> `bashy go build` (exec'd WITHOUT BASHY_ADAPTER_INPROC) -> toolchain/go/go.bsh -> its own cache is stale (the checkout is newer) -> `go build` -> `bashy go build` -> go.bsh -> ... The adapter template's `command -v go` finds the host go, yet under agent mode the `go` exec is served as `bashy go` without the in-process escape hatch. A `VAR=1 cmd` prefix and the INPROC fallback both propagate correctly (checked).

Cause: product defect. Agent-mode exec routing sends a bare `go` inside an adapter script to the bashy `go` verb, and nothing bounds re-entry into toolchain/go/go.bsh. Not yet bisected: whether 86c645bc (env strip) or earlier adapter work introduced it.

Fix direction (smallest robust): (1) a recursion guard: a toolchain/go adapter rebuild must never dispatch back into toolchain/go (for example, go.bsh always uses `BASHY_ADAPTER_INPROC=1 "$self" go` for its own build); (2) find the agent-mode route that turns a bare `go` into `bashy go`, and make adapter scripts (BASHY_ADAPTER_EXEC) exempt; (3) a unit test: an agent-mode cold build of one yoke adapter with an empty cache finishes with a bounded process count.

Workaround used: warm the adapter cache once without agent mode (`BASHY_AGENTIC=0 bin/bashy judge --help`) before `installbashy`.
