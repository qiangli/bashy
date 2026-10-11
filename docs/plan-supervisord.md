# `bashy supervisord` — from one dag root to the service monitor (Sprint 218, retired by Sprint 418 S19)

**Status: the `DAG.md TARGET` form shipped 2026-09-20 and was retired by S19 (2026-10-11).**
`bashy supervisord` is now the service monitor (S16): design in
[`bashy-service-design.md`](bashy-service-design.md). Code:
`internal/agentos/supervisord*.go`. Atlas: `bashyOwnedVerbAtlas["supervisord"]` (deploy ·
orchestration · workspace, `spawns-processes` + `daemon`, effect `exec`, partial on Windows).

## What replaced the command

```
bashy supervisord DAG.md TARGET            # retired: exit 2, prints the unit that replaces it
bashy supervisord [--pid1] [--until-done]  # the monitor
```

A dag root is a unit whose exec is `bashy dag DAG.md TARGET`. The Sprint 218 flags became unit
fields; the behaviour became unit semantics under the one monitor policy:

| Sprint 218 | Now |
|---|---|
| `--min-backoff 1s` / `--max-backoff 30s` / `--healthy-after 30s` | `restart.backoff.min` / `max` / `healthy_after` |
| `--grace 10s` | `stop.timeout` |
| root exits 0: supervisor exits 0 | the unit completes and is not restarted; `--until-done` ends the monitor |
| non-zero exit: TERM the old group, back off, restart | restart policy; the old group is deactivated before each restart |
| TERM / INT forwarded to the whole group, KILL after the grace | the stop signal to each unit's group, `stop.timeout`, then KILL |
| TERM / INT during the backoff: exit 0 | unchanged, for every unit |
| PID 1 / subreaper reaping | unchanged: `--pid1`, the arm/watch/take stash |
| `dag --json -n` preflight; a permanent error never restarts | gone; the start limit (5 in 5m) leaves a hopeless root **failed** |

## The refusal

`bashy supervisord DAG.md TARGET` (and `--min-backoff`, `--max-backoff`, `--healthy-after`,
`--grace`, which only that form had) exits 2 and prints the `root.yaml` for the invocation, with
its operands and flag values filled in, then `bashy service add -f root.yaml`,
`bashy supervisord --pid1` and `bashy service run root`. Nothing is run.

## The one race, and how it is closed (Linux)

A PID-1 reaper does `wait4(-1)`. It can collect a unit's process before `os/exec.Wait` does, and
then `Wait` fails with `ECHILD` and the exit status — the one fact restart policy needs — is
gone. Rather than peeking at `siginfo` layouts (`waitid(WNOWAIT)`), the reaper keeps what it took:
`arm()` before a spawn empties its stash and stashes *every* pid reaped until `watch(pid)` names
the live child (the spawn window is microseconds, and a stale pid cannot be the live child's),
after which only watched pids are kept and everything else is counted as an orphan.
`execChild.Wait` consults `take(pid)` when its own wait comes back empty. With many units there
are many watched pids at once (`TestReaperKeepsEveryLiveMainChild`). Same contract in and out of a
container: the monitor becomes a subreaper whenever it can, which is also what makes the reaping
testable on a plain Linux host (`TestServiceMonitorReapsOrphansAsSubreaper`).

Prior art adapted, none copied: Outpost's restart/backoff/signal model; ochinchina/supervisord's
`autorestart=unexpected` and its `ReapZombie` loop; u-root `libinit.WaitOrphans`.

## Non-Linux

- **macOS / BSD:** process groups and signal forwarding are the same; there is no subreaper and
  the process is never PID 1, so `--pid1` is refused and no reaper runs.
- **Windows:** no POSIX groups or signals. The child gets `CREATE_NEW_PROCESS_GROUP`, the
  graceful phase is a `CTRL_BREAK` event to that group, the kill reaches the direct child only,
  and there is no reaping. The atlas record says so (`Partial: windows`).

## Naming

`bashy supervisord` is a **direct front-door verb** (catalogued, callable, never a bare shim):
`supervisord` is a widely installed program name and bare `supervisord` must keep resolving to
the host's, exactly as `ping` and `awd` are handled.

## Tests

The Sprint 218 tests were ported, not dropped (`supervisord_test.go`): the fake-provider and
fake-clock policy tests (completion, the 1·2·4·8·8 → healthy → 1 backoff sequence, signal during
backoff), then the test binary re-execs itself as the scripted stand-in for `bashy dag` (argv and
DAG path, real restarts until exit 0, the start limit on a root that never succeeds, TERM to a
real process group, KILL of a child that ignores TERM), and — Linux — orphan reaping as a
subreaper with the unit's own status intact. New: the refusal and the unit it prints.
