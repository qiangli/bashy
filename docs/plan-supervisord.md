# `bashy supervisord` — one dag root, supervised, in the foreground (Sprint 218)

**Status: shipped 2026-09-20.** Code: `internal/agentos/supervisord{,_unix,_linux,_other,_windows}.go`;
tests beside it. Atlas: `bashyOwnedVerbAtlas["supervisord"]` (deploy · orchestration · workspace,
`spawns-processes` + `daemon`, effect `exec`, partial on Windows).

## The command

```
bashy supervisord [--min-backoff 1s] [--max-backoff 30s] [--healthy-after 30s] [--grace 10s] DAG.md TARGET
```

It runs `bashy dag DAG.md TARGET` — the **current executable**, so the root runs the same
bashy that supervises it — as the leader of its own process group, with the supervisor's
stdin/stdout/stderr inherited, and keeps it running:

| event | action |
|---|---|
| root exits 0 | the root completed; supervisor exits 0 |
| root exits non-zero / dies of a signal we did not send | TERM the old group, sleep `backoff`, KILL what is left, restart |
| restart delay | `min · 2ⁿ` capped at `max`; a run that lasted `--healthy-after` puts the ladder back at `min` |
| TERM / INT to the supervisor | forwarded to the **whole group**; after `--grace` (or a second signal) the group is KILLed; supervisor exits 0 |
| TERM / INT during the backoff sleep | exit 0 without restarting |
| PID 1 on Linux, or any Linux host where `PR_SET_CHILD_SUBREAPER` is granted | adopted orphans are reaped |

Startup runs one **preflight**: `bashy dag --json -n DAG.md TARGET`. An unknown target or an
unparseable file is reported with dag's own diagnostic and exit code and is *never* restarted —
a restart loop over a permanent error is the failure mode this closes. The plan it returns is
logged (`plan: a -> b -> svc`), so the `Requires:` closure — the **only** dependency format —
is visible without the supervisor ever parsing the file itself.

## The one race, and how it is closed (Linux)

A PID-1 reaper does `wait4(-1)`. It can collect the *main* child before `os/exec.Wait` does,
and then `Wait` fails with `ECHILD` and the exit status — the one fact restart policy needs —
is gone. Rather than peeking at `siginfo` layouts (`waitid(WNOWAIT)`), the reaper keeps what it
took: `arm()` before a spawn empties its stash and stashes *every* pid reaped until `watch(pid)`
names the live child (the spawn window is microseconds, and a stale pid cannot be the live
child's), after which only that pid is kept and everything else is counted as an orphan.
`execChild.Wait` consults `take(pid)` when its own wait comes back empty. Same contract in and
out of a container: the supervisor becomes a subreaper whenever it can, which is also what makes
the reaping testable on a plain Linux host (`TestSupervisordReapsOrphansAsSubreaper`).

Prior art adapted, none copied: Outpost's restart/backoff/signal model; ochinchina/supervisord's
`autorestart=unexpected` and its `ReapZombie` loop; u-root `libinit.WaitOrphans`.

## Non-Linux

- **macOS / BSD:** process groups and signal forwarding are the same; there is no subreaper and
  the process is never PID 1, so no reaper runs — nobody's orphans are ours.
- **Windows:** no POSIX groups or signals. The child gets `CREATE_NEW_PROCESS_GROUP`, the
  graceful phase is a `CTRL_BREAK` event to that group (a Go child sees `SIGINT`; the child
  must share our console for it to arrive), the kill reaches the direct child only, and there
  is no reaping. The atlas record says so (`Partial: windows`).

## Deliberately out (KISS)

No daemon mode, pidfile, status/control socket, HTTP/RPC/UI, log files, config file or
directory, service scripts, file watcher, multiple programs, readiness protocol, mounts, TTY
setup, or initramfs/kernel boot. A host that needs those has outpost; this verb is what a
container `ENTRYPOINT` or a `systemd` `ExecStart=` line points at.

## Naming

`bashy supervisord` is a **direct front-door verb** (catalogued, callable, never a bare shim):
`supervisord` is a widely installed program name and bare `supervisord` must keep resolving to
the host's, exactly as `ping` and `awd` are handled.

## Tests

Fake child + fake clock + fake signal source drive the loop with no sleeping: completion,
the 1·2·4·8·8 → healthy → 1 backoff sequence, TERM forwarded and honoured, grace expiry →
KILL, signal during backoff. Then the test binary re-execs itself as a scripted stand-in
for the dag root: argv/DAG-path propagation and the reported Requires order, inherited
stdout, real restarts until exit 0, a real preflight refusal (skips without a `bashy` on
PATH), TERM to a real process group, KILL of a child that ignores TERM, and — Linux —
orphan reaping as a subreaper with the root's own status intact.
