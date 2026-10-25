# Sprint 407 Story 1951: agent external-command latency

## Diagnosis

`CLAUDECODE=1` enables execution history. A macOS `sample` profile of the
installed bashy placed the active command path in
`execHistHandler → recorder.observeSpace → spacegraph.Store.Record → Live → all`.
Each `/usr/bin/true` replayed and JSON-decoded the entire graph log (213 MB on
Dragon). Turning off `BASHY_EXECHIST` removed the delay; turning off claim
checks did not. Agent detection itself is not the expensive operation.

The `perf`-tagged `TestAgentExternalCommandLatency` seeds an isolated large
graph log and times eight external commands with and without `CLAUDECODE=1`.
It failed before the yoke change at 523.73 ms added per command on macOS.

## Change

`yoke/pkg/spacegraph.Record` now appends one delta observation without reading
the log. `Live` folds deltas with existing full-snapshot records, preserving
counts, first/last timestamps, the latest account and coordinates, and
supersession. This makes writes independent of log size even for an edge that
has never been seen before. The Bashy integration test has a 10 ms per-command
budget. The conductor publishes the yoke commit, then bumps Bashy's yoke pin.
An older reader does not understand delta records and may show understated
counts during a mixed-version rollout, so the reader and writer should be
deployed together.

## Measurements and gates

| Host and fixture | Baseline with agent marker | Fixed with agent marker | Fixed without marker |
| --- | ---: | ---: | ---: |
| macOS Dragon, isolated ~96 MB graph log | +523.73 ms/command | +6.90 ms/command | paired reference |
| Linux arm64 Lima VM, isolated 107.5 MB graph log | 683.12 ms/command | 3.16 ms/command | 2.87 ms/command |

The macOS figures are paired marker overhead, while the Linux figures are
median total wall time per command across three eight-command runs. The Linux
fixed marker overhead was 0.29 ms per command.

`make test-bash` completed with 86 passed, 0 failed, 0 skipped, 0 timed out.
Yoke's focused `pkg/spacegraph` tests and Bashy's execution-history, claim and
advisor tests passed against the local yoke worktree. Yoke's full
`scripts/crossvet.sh` could not complete from that isolated worktree because
`external/ollama/src` is not hydrated; the AIX canary also fails in the pinned
shell and go-billy dependencies. Focused Darwin, Linux and Windows vet of
`pkg/spacegraph` passed.
