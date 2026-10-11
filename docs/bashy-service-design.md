# bashy service and the bashy supervisord monitor (Sprint 418 S16, S19)

**Status:** S16 implemented in `internal/agentos/supervisord*.go`, 2026-10-11. S19
(the dag root becomes a unit; the `DAG.md TARGET` form is refused) landed on top.

**Contract of record:** yoke `docs/bashy-service-design.md` (S15). It covers
the unit schema, the rings and seeds, the port rule, the `pkg/service`
provider contract, and the retained-coverage inventory. This file records
what S16 builds on that contract inside bashy: the monitor, the process
provider, the control socket and the runtime verbs. The dhnt umbrella's
`docs/bashy-service-design.md` (S14) remains the design of record for the
whole sprint.

## Roles: the inetd/systemd split

| Role | Unix analogue | Here |
|---|---|---|
| Unit definitions and per-unit verbs | unit files + `systemctl` | `bashy service` — yoke `fleet.NewServiceCmd` mounted as a direct front-door verb, plus the S16 runtime verbs |
| Service monitor | `inetd` | `bashy supervisord` — a bashy command, long-running, foreground |
| Init | `systemd` | `outpost supervisord` starts and watches only the outpost daemon and `bashy supervisord` (S18) |

A container needs no outpost. Its entrypoint is `bashy supervisord --pid1`.

`service` is a **direct-only** front door (`directFrontDoorVerbs`). Bare
`service` stays the host's service(8), just as bare `supervisord` stays the
Python supervisord.

## Rod, not fish

There is one mechanism, and nothing in it branches on a unit's name.

- **Desired state** is the registry: `fleet.Catalog.Services()` across the
  embedded seeds and the shared and local rings.
- **The provider is picked by the declaration's `provider` field** through
  `service.Providers.For`.
- **One policy applies to every unit**, read from its YAML through
  `unitPolicy`: restart, backoff, start limit, probe, stop signal, grace and
  `requires`.
- **One status shape:** `service.Status`.
- **One event shape:** `service.Event`, published as `service.<state>`.

A seeded built-in such as `schedule`, an internal capability, and a fresh
`bashy service add -f` declaration therefore use the same verbs, the same
monitor and the same status. An operator override (`bashy service set NAME
…`, a sparse overlay) is just a changed declaration. The monitor diffs it on
reload and applies it to that unit only.

`TestServiceMonitorSeedInternalAndCustomShareOneMechanism` exercises exactly
this:

- a seed enabled by `service enable`;
- a custom unit added by `service add -f`;
- an internal unit hosted by the custom unit's process and applied through
  yoke's `service.Internal`;
- an argv override with `service set`;
- `stop` and `start`, and a live `disable`.

It asserts on pids that no bystander is restarted. The test contains no
unit-name branches.

## The monitor

`bashy supervisord` takes no operands and runs the monitor. The Sprint 218 form
`bashy supervisord DAG.md TARGET` is retired (S19) and refused; see
[A dag root is a unit](#a-dag-root-is-a-unit).

```
bashy supervisord [--pid1] [--until-done] [--socket PATH] [--poll DUR]
```

### Concurrency model

- A single loop goroutine owns every `unitRun`.
- Blocking provider calls run on their own goroutines and post messages
  back: activate, deactivate (which waits out the grace), configure, and
  probes.
- Every message carries the unit's **generation**. A late answer about an
  incarnation that has since been replaced is dropped.
- Timeline and notify delivery happen off the loop, so a slow sink never
  stalls supervision.

### Unit lifecycle

`step()` moves each unit one action toward its desired state, repeated until
nothing changes. Units are visited in name order, which keeps the result
deterministic.

| From | Condition | Action / state |
|---|---|---|
| inactive | wanted up, every `requires` (plus an internal unit's owner) active or degraded | start limit check → foreign port check (process units) → resolve runtime → `Activate` → **starting** |
| inactive | a dependency is disabled, stopped, unknown or failed | **failed** with that reason. Not latched: the unit starts once the dependency is back. |
| inactive | a dependency is only on its way up | stays inactive with reason `waiting for X` |
| starting | probe passes (or, with no declared probe, `Provider.Health` is nil) | **active** |
| active | probe fails | **degraded**. After `health.failures` consecutive failures the unit is restarted. `start_period` failures do not count while starting. |
| running | provider reports an exit | quiet cleanup (`Deactivate` kills the rest of the group and closes the log), then the restart policy |
| running | no longer wanted (disable, stop, rm, retire, shutdown) | once no running unit requires it: **stopping** → `Deactivate` (stop signal, grace, KILL) → **inactive** |
| running | the declaration changed | `Configure`. `ErrRestartRequired` triggers a restart that is not counted against the start limit. |

### Restart policy

The restart policy is `restart.policy`, with `on-failure` as the default:

- `always` restarts every exit;
- `on-failure` restarts a non-zero or signalled exit, and a clean exit is
  **completed** (inactive, latched);
- `no` never restarts, and a failure is **failed**.

Delays come from `backoffLadder`, the same type the dag-root mode now uses:

- it starts at `backoff.min`;
- it doubles on each consecutive failure, up to `backoff.max`;
- it resets after a run that lasted `healthy_after`.

### Start limit

`startLimiter` allows `start_limit.burst` starts within `interval` (default
5 in 5m). The next start moves the unit to **failed** with the reason
`start limit hit: N starts within W`. The unit then stays down: there is no
infinite crash loop.

The latch on a failed or completed unit is cleared by any of:

- `bashy service start` or `restart`, which also resets the limiter;
- a changed declaration (`set` or `add`);
- re-enabling the unit.

### Foreign listener

Before a process unit starts, every declared port is checked. The check
first connects to the port, because on BSD and macOS a wildcard listener
does not block a `SO_REUSEADDR` bind of a specific address. It then tries to
bind.

The registry already refuses two *enabled* units on one port. A held port
is therefore a process the monitor does not own, and the unit becomes
**failed** with:

```
port 8637/tcp (127.0.0.1:8637) is held by a foreign listener
```

Retrying cannot evict another program, so the monitor does not retry.

### Live reload

The monitor reloads the registry in three cases:

- when a `bashy service` write verb (`add`, `set`, `rm`, `edit`, `enable`,
  `disable`, `retire`, `unretire`) succeeds, the CLI asks the running
  monitor to reload over the socket;
- every `--poll` (default 5s), which catches edits made by other tools;
- on the socket's `reload` op.

A reload compares declaration fingerprints. A unit whose declaration did not
change is not touched, so enabling or disabling one unit never restarts
another (`TestMonitorLiveEnableDisableLeavesOtherUnitsAlone`).

### Shutdown and `--pid1`

On TERM or INT, every unit is stopped, dependents before what they require,
and the monitor exits 0. A second signal cancels every remaining grace
period.

The orphan reaper is the Sprint 218 one, generalized from one main child to
a set:

- `arm` and `watch` bracket each spawn;
- the spawn window is a counter;
- `take` forgets the pid;
- exec probes spawn through the same `startTracked`.

So a status the reaper collects first still reaches the right `Wait`.

The monitor always becomes a subreaper when it can. `--pid1` makes that
mandatory and refuses to start where it is impossible (not PID 1, and not
Linux).

## The process provider

`processProvider` implements `service.Provider` for `provider: process`.

- **Foreground only:** no daemonizing and no pidfile. The child is the
  leader of a new process group (`ownProcessGroupAttr`). Stop sends the
  declared signal to the group, waits `stop.timeout` (default 10s), then
  sends KILL.
- **argv:** `ExpandExec` resolves the placeholders. A leading `bashy` is
  this executable, so a monitor never runs a different bashy from PATH.
  `workdir` expands `~`.
- **Runtime** (`resolveUnitRuntime`): `env`, plus every `secrets[]` ref
  resolved through `secrets.ResolveAgentKey`. `download` is provisioned by
  `fleet.Command{Download}.Ensure`, the same verified binmgr path a
  registered command uses.
- **Logs:** stdout and stderr go to `BASHY_HOME/service/NAME/output.log`.
  At 10 MiB the file rotates, keeping `.1` to `.3`. Supervisor start and
  exit lines are interleaved, and `cmd.WaitDelay` bounds a grandchild that
  holds the pipe open. `bashy service logs NAME [-n N] [-f]` reads the log.
- **`Configure`** compares the runtime fingerprint (argv, env and workdir).
  Any difference returns `ErrRestartRequired`. Policy-only changes (health,
  restart, expose) apply live.
- **Exit reporting:** an exit nobody requested is reported through
  `Runtime.Emit` as `To: inactive`, with the code in `Status.LastExit`. The
  rest of the group is sent TERM, so a straggler cannot hold the port the
  restart needs.

## Internal units: the delegated provider

An internal unit's capability lives in its owner's process: outpost, or the
unit hosting it, such as meet for `events`. The monitor still owns the
unit's policy, and `delegatedInternal` is its provider:

- **`Activate`** marks the unit wanted. Its state is **starting** with the
  reason `awaiting owner X`, and `Health` returns `errHealthPending`. That
  error means "not yet" and does not count as a probe failure.
- **The owner applies the unit** with yoke's `service.Internal` and its
  capability `Registry`:
  1. it polls `desired` for its owner name;
  2. it activates, configures or deactivates the capability;
  3. it sends `report` with the resulting `service.Status`.
- **A report** updates status and health. A change of state reaches the
  monitor as a provider event.
- **The unit's state is the capability's state.** There is no pid, no
  wrapper child and no feature flag.
- **The owner is an implied dependency** (`Dependencies()`). When the owner
  stops, its internal units go down; when the owner comes back, they wait
  for it to apply them again.

Wiring outpost and meet as owners is S18. The protocol above is the
contract they code against.

## Control socket and status

The socket is at `BASHY_HOME/service/supervisord.sock`, overridable with
`BASHY_SUPERVISORD_SOCKET` or `--socket`. It is mode 0600. Each connection
carries one JSON request and one JSON response. A second monitor refuses to
start while a live one answers.

| op | request | response |
|---|---|---|
| `status` | `unit?` | `units: [service.Status]` |
| `start` / `stop` / `restart` | `unit` | `units: [service.Status]`. `start` runs even a disabled unit until `stop` (runtime only). `stop` holds the unit down until `start`. Neither persists. |
| `reload` | none | re-reads the registry |
| `desired` | `owner` | the internal units that owner hosts. `enabled` means "apply it now". |
| `report` | `status` | the owner's view of one unit |

`bashy service status [NAME…] [--json]` prints `bashy-service-status-v1`:

```json
{"schema_version": "bashy-service-status-v1",
 "monitor": {"running": true, "pid": 1234, "socket": "…/supervisord.sock"},
 "units": [ service.Status … ]}
```

The golden file is `internal/agentos/testdata/service-status.golden.json`
(`TestServiceStatusJSONGolden`). Regenerate it with `BASHY_UPDATE_GOLDEN=1`.

The monitor overlays the provider's pid and last exit, the declaration's
ports, and (only while `State.Exposed()`) the `exposed` app and mesh names.
Outpost's exposure reconciler reads exactly this, and nothing parses status
text.

With no monitor running, status reports each unit's desired state as
`inactive` with the reason `supervisord is not running`, and does not guess.

## Events and notification

Every transition is a `service.Event`:

- **Timeline (S3):** it is appended to the room timeline with `room.Record`.
  The type is `service.<state>`, the target `service:NAME`, the topic the
  previous state, and the body the reason.
- **Notify sinks (S7):** `failed` is additionally published with
  `room.Notify`, principal `bashy-supervisord` and topic `service`.

Both sinks are monitor fields, so S7 can replace the notify path without
touching policy.

## Runtime verbs

| Verb | Needs the monitor | What it does |
|---|---|---|
| `service status [NAME…] [--json]` | no (degrades) | structured state |
| `service start\|stop\|restart NAME` | yes | runtime-only control. Without a monitor the error names `bashy supervisord` and `service run`. |
| `service logs NAME [-n N] [-f]` | no | the unit's rotating log |
| `service run NAME` | no | one unit in the foreground, under the same monitor code and policy, enabled or not. Output goes to this terminal, there is no socket, and its `requires` are not started. Exits 0 when the unit completes or on TERM/INT, and 1 when it ends failed. |

## Acceptance evidence

All tests are in `internal/agentos`.

| Acceptance | Test |
|---|---|
| Backoff doubles, caps and resets after a healthy run | `TestBackoffLadderDoublesCapsAndResetsAfterHealthyRun`; the same sequence on a dag-root unit, `TestDagRootUnitBackoffDoublesCapsAndResetsAfterHealthyRun` |
| Start limit leaves the unit failed and notifies once; `start` resets it | `TestStartLimiterRefusesBeyondBurstWithinWindow`, `TestMonitorStartLimitMovesCrashLoopToFailed` |
| Health failure restarts the unit | `TestMonitorHealthFailureRestartsWithProbe` |
| Foreign listener reported as failed, with the reason | `TestMonitorForeignListenerOnUnitPortIsFailedWithReason` |
| Live enable, disable and reconfigure; no bystander restarts | `TestMonitorLiveEnableDisableLeavesOtherUnitsAlone` |
| `requires` ordering | `TestMonitorRequiresOrdering` |
| PID1 / subreaper reaping on Linux; many main children | `TestServiceMonitorReapsOrphansAsSubreaper`, `TestReaperKeepsEveryLiveMainChild` (`supervisord_linux_test.go`) |
| Status `--json` schema golden | `TestServiceStatusJSONGolden` |
| Built-in, internal and custom units through the same verbs, plus an operator override | `TestServiceMonitorSeedInternalAndCustomShareOneMechanism` |

## A dag root is a unit

Sprint 218's `bashy supervisord DAG.md TARGET` was a second supervisor beside
the monitor. S19 removes it. A dag root is an ordinary unit whose exec is
`bashy dag DAG.md TARGET`:

```yaml
name: root
kind: service
enabled: true
exec: [bashy, dag, /srv/DAG.md, root]
restart: {backoff: {min: 1s, max: 30s, healthy_after: 30s}}
stop: {timeout: 10s}
```

A leading `bashy` in `exec` is replaced by the monitor's own executable, so the
root runs the same bashy that supervises it. The unit is added with
`bashy service add -f root.yaml`, or shipped read-only in a directory on
`$BASHY_SERVICES_PATH`; `bashy service run root` runs just that unit in the
foreground (the replacement for running the old form by hand).

**The old form is refused.** `bashy supervisord DAG.md TARGET`, and the flags
only it had (`--min-backoff`, `--max-backoff`, `--healthy-after`, `--grace`),
exit 2 and print the `root.yaml` that replaces the invocation, with the
operands and any flag values already filled in, plus the commands to use it.
Nothing is run.

**The Sprint 218 contract is unit semantics**, proven by the old tests ported
onto a dag-root unit (`supervisord_test.go`):

| Sprint 218 | Unit field or behaviour | Test |
|---|---|---|
| exit 0 completes; supervisor exits 0 | a clean exit completes the unit; it is not restarted | `TestDagRootUnitCompletesOnExitZero` |
| `--min-backoff/--max-backoff/--healthy-after` | `restart.backoff.min/max/healthy_after` | `TestDagRootUnitBackoffDoublesCapsAndResetsAfterHealthyRun` |
| restart a failing root; group cleaned up first | restart policy; `Deactivate` before every restart | same, and `TestDagRootUnitRestartsARealChildUntilItCompletes` |
| TERM/INT forwarded to the whole group | the declared stop signal to the unit's group | `TestMonitorForwardsTermToARealProcessGroup` |
| `--grace`, then KILL | `stop.timeout`, then KILL | `TestMonitorKillsARealChildThatIgnoresTerm` |
| signal during the backoff sleep exits without restarting | the same, for any unit | `TestMonitorSignalDuringBackoffExitsWithoutRestarting` |
| PID 1 reaping without losing the root's status | `--pid1` / subreaper reaper | `TestServiceMonitorReapsOrphansAsSubreaper` |
| argv, DAG path and stdio reach the root | `exec` argv; output in the unit log | `TestDagRootUnitPropagatesArgvAndInheritsOutput` |
| the old form is refused | exit 2 with the unit form | `TestSupervisordRefusesTheRetiredDAGForm`, `TestSupervisordRefusalNamesTheUnitForm`, e2e `TestE2EAllListedCommandsDispatch` |

**What changed on purpose.**

- The `dag --json -n` preflight is gone. A root that can never succeed (an
  unknown target, an unparseable file) is bounded by the start limit (5 starts
  in 5 minutes by default) and left **failed** with `start limit hit`, rather
  than refused before the first start. `TestDagRootUnitThatNeverSucceedsEndsFailedAtTheStartLimit`.
- The root's output goes to the unit log (`bashy service logs root`), not the
  monitor's own stdout.
- `--until-done` ends the monitor once every enabled unit has completed (exit
  0) or failed (exit 1). It is how a one-shot root, such as the scratch-image
  smoke, ends its container; a long-running root never trips it.

**The scratch image.** The FROM-scratch image's entrypoint is
`/bashy supervisord --pid1`. The root unit and its DAG are data in the image
(`/etc/bashy/services/root.yaml` on `BASHY_SERVICES_PATH`, `/srv/DAG.md`);
`BASHY_HOME` is a writable volume for logs and the control socket.
`scripts/verify-bashy-scratch.sh` builds exactly that image, runs it with
`--until-done`, reads the root's output back with `service logs root`, and
checks that the old form is refused inside the image.

## Out of scope here

- **S17:** replacing the private start/stop copies (svcd, meet, sdlc,
  schedule, loom, the llm door) with these units.
- **S18:** outpost init, outpost and meet as internal-capability owners, and
  the exposure reconciler and admin toggles re-pointed to this socket.
