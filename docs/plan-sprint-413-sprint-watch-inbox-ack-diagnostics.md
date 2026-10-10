# Sprint 413 — sprint watch / inbox-ack diagnostics (bashy side)

Stories: **#1811 `4a18c41ef997`** (start/take `--watch` exited 1 silently; lease then
rendered an absurd stale duration) and **#1834 `491a03436cbb`** (`sprint inbox-ack`
exited 1 silently for an external manager; unread count stayed until an
`inbox --watch` read consumed it).

Worker `claude-opus5-w14`, weave run bashy#14, branch `agent/weave-issue-14`.
Scope boundary: **changes land in bashy only.** The one defect that lives in
canonical yoke is reported below as a patch for the manager, not applied.

## Diagnosis

### 1. The silence — one root cause, reported twice

`weave.NewSprintCmd` sets `SilenceErrors`/`SilenceUsage` on **every** command in
the sprint tree (a subverb prints its own structured envelope and cobra must not
double-print on top of it), and compensates by installing three reporters while
it builds the tree:

| yoke file | covers |
|---|---|
| `pkg/weave/flagerr.go` | flag-parse failures |
| `pkg/weave/argerr.go` | positional-argument failures (`ValidateArgs`) |
| `pkg/weave/runerr.go` | errors a `RunE` returns itself |

Everything those report comes back as a **structured exit**, and
`weave.IsStructuredExit` is how a host is told "already printed, stay quiet".

bashy then **edits that finished tree** in `newSprintCmd` (`internal/agentos/sprint.go`):

- `attachSprintWatch` **replaces** `start`'s and `take`'s `RunE` with a closure
  that calls the reported original inside itself. The closure's **own** guards —
  the sprint-id parse, `weave.SprintClaimIdentity`, `registerSprintInboxWatcher`,
  and the whole `runSprintInboxWatch` loop — sit *outside* every reporter.
- `inbox-ack`, `monitor` and `wait` are **added after** yoke's reporters already
  walked the tree, so their `RunE`s were never wrapped at all.

Those paths were therefore reported by nobody, and the host
(`case "sprint"` in `agentos.go`) mapped every `Execute()` error to a bare
`dispatchExit(1)` — it never called `IsStructuredExit`/`ExitCode`, which the
yoke doc comment explicitly asks a host to do.

Result, measured: **exit 1 with zero bytes on both streams.** This is exactly
why the symptom looked like "`--watch` fails and plain `start` succeeds" — plain
`start` reaches yoke's reporting `RunE` and is loud; `--watch` is the only way to
reach bashy's unreported guards.

### 2. `inbox-ack`'s message was no better than the silence

Underneath the silence, one sentence covered four distinct causes
(`room.Find` failed · no card · wrong mode · missing capability), and
`room.Find`'s own error was discarded by `if err != nil || !live || …`. It never
named the command that actually consumes the mail — Sprint 329's manager found
`bashy inbox --as s329-manager --watch` by guessing. The `--help` named it either.

### 3. The watch's *normal* exits were silent too

`runSprintInboxWatch` returns **nil** on ctrl-C, SIGTERM, a cancelled context and
a closed poll wait. So a conductor whose seat had just been stood down saw a
clean `exit 0` and no reason — as uninformative as the failure. The seat's
`release` defer then zeroes the lease heartbeat, which is what produces §4.

### 4. The absurd stale duration is in canonical yoke — reported, not fixed

Confirmed live on this candidate (raw log
`/tmp/s413-w14-logs/native-lifecycle.log`, step F):

```
  conductor:  s413-probe (STALE (no heartbeat for 2562047h47m0s — take it))
```

`2562047h47m0s` is `time.Duration`'s saturated max: it is
`time.Since(time.Time{})`, not an age.

Chain: `weave.ReleaseSprintManagerLease`
(`yoke/pkg/weave/weave_story_identity.go:104`) stands a seat down by setting
`s.Lease.At = time.Time{}` while **keeping** `Lease.Holder`. `weaveStoryLeaseState`
then reports `stale=true, free=false` (correctly — `role.Seat.Live` returns
`LivenessUnknown` for a zero heartbeat), and the renderer at
`yoke/pkg/weave/weave_story.go:867` formats `time.Since(s.Lease.At)` regardless.
The same record is produced by the dead-`AttachedPID` path at
`weave_story.go:218`, which withdraws the heartbeat on purpose.

`role.Liveness` **already** separates the two verdicts, and
`weaveStoryLeaseState`'s own comment invites a caller that can act on the
difference to ask `seat()` directly — so the fix is a renderer fix, not a model
change. Note this is now *more* reachable than before: bashy's explained-exit
work means a clean detach reliably stands the seat down, so every orderly watch
exit lands on this line.

#### Minimal patch for the manager (yoke, `pkg/weave/weave_story.go`)

Baseline: yoke working tree at `b8f05aa09a3aa0d18491043e67c852b0330684f8`
(line numbers verified there). No other render site exists —
`grep -rn "no heartbeat" pkg/weave pkg/*/` finds this one plus prose.

```diff
@@ pkg/weave/weave_story.go:864 @@
 	if h, stale, free := weaveStoryLeaseState(s); !free {
 		st := "fresh"
 		if stale {
-			st = fmt.Sprintf("STALE (no heartbeat for %s — take it)", time.Since(s.Lease.At).Round(time.Minute))
+			// A ZERO BEAT IS NOT AN OLD BEAT. ReleaseSprintManagerLease stands a
+			// seat down by clearing Lease.At, and seat() withdraws the heartbeat
+			// outright when a named attached process is dead — so on both paths
+			// time.Since(Lease.At) is not an age, it is the distance from the
+			// zero instant, and it rendered as "no heartbeat for 2562047h47m0s"
+			// on Sprint 412's board (todo 4a18c41ef997). role.Seat.Live already
+			// returns LivenessUnknown rather than LivenessLapsed here; the only
+			// thing missing was saying which verdict this is.
+			switch {
+			case s.seat().Live(time.Now()) == role.LivenessUnknown:
+				st = "STALE (no live heartbeat — the seat was stood down, its watch died, " +
+					"or it never beat; look before you seize it, then `sprint take`)"
+			default:
+				st = fmt.Sprintf("STALE (no heartbeat for %s — take it)", time.Since(s.Lease.At).Round(time.Minute))
+			}
 		}
 		fmt.Fprintf(out, "  conductor:  %s (%s)\n", h, st)
```

`role` is already imported in this file (`s.seat()` returns `role.Seat`). The
`s.Lease.At.IsZero()` test is the one-line variant, but asking `seat()` also
covers the dead-PID and future-heartbeat cases that `IsZero` misses, and it keeps
one liveness rule rather than two.

Suggested yoke regression (same package, no new fixture):

```go
// A stood-down seat must not render a 292-year heartbeat age.
func TestSprintShowRendersAZeroHeartbeatAsUnknownNotAnAge(t *testing.T) {
	// card held by "m", Lease.At = time.Time{}
	// want: no "2562047h", and the word "no live heartbeat"
}
```

`sprint tick`/`sprint monitor` need no change — bashy has no second render site
(`grep -rn "time.Since" internal/agentos/*.go` confirms).

## What changed in bashy

| File | Change |
|---|---|
| `internal/agentos/sprint_errors.go` (new) | the bashy half of the self-reporting contract: `installSprintErrorReporting` walks the tree **after** every bashy edit and wraps each `RunE` so a bare error becomes one message on the command's own stderr plus a `*sprintReportedError` carrying its exit code. Structured errors pass through untouched, which is what keeps the count at exactly one. `sprintUsage`/`sprintUsagef` mark invocation-shaped failures so they get yoke's exit 2 instead of being guessed from message text. |
| `internal/agentos/sprint.go` | call `installSprintErrorReporting(cmd)` last in `newSprintCmd`; classify `attachSprintWatch`'s three guards; explain that a failed stream registration means the seat was **not** claimed, and why dropping `--watch` is the wrong workaround. |
| `internal/agentos/agentos.go` | `case "sprint"` extracted to `runSprintDispatch(args, stdout, stderr) int` (peer of `runDagDispatch`, and testable), which now honours `sprintErrorReported` → `weave.IsStructuredExit` → print, and returns `weave.ExitCode`. |
| `internal/agentos/sprint_watch.go` | `runSprintInboxWatch` split into a wrapper plus `runSprintInboxWatchLoop` returning a named `sprintWatchDetachCause`; every return site carries its cause and a prefixed error; `writeSprintWatchDetach` writes one stderr line per exit naming the cause, that the seat is no longer held, and that **no mail was lost** plus the read that recovers it. `requireAttachedSprintWatch` extracted from `inbox-ack`: reports `room.Find`'s own error separately, distinguishes no-card from wrong-mode, compares mode case-insensitively, and ends every refusal with the consuming read. `inbox-ack` gained a `Long` help that names `bashy inbox --as NAME [--watch|--peek]` as the reads that consume. |
| `internal/agentos/inbox.go` | `errInboxWatcherLive` now names the holder (pid, owning session pid, mode, task) and how to look / stop / read without a watcher. Best-effort: the refusal never depends on the lookup. |

Nothing weakens the existing guarantees: unacknowledged input is still never
consumed, the watch still reminds forever rather than quitting, the lease is
still beaten at `SprintLeaseTTL/3`, epoch/generation fencing is untouched, and
the seat is still stood down on every exit.

## Verification

Raw logs under `/tmp/s413-w14-logs/`.

| Log | What |
|---|---|
| `red-sprint-dispatch.log` | **RED**: `start --watch`, `take --watch`, `inbox-ack` each "exited 1 with ZERO output on both streams"; `unknown subcommand` already passed, isolating the gap to bashy's own edits. |
| `red-inbox-ack-diagnosis.log` | **RED**: the old one-line refusal, with every actionable string missing and exit 1. |
| `red-sprint-watch-exits.log` | **RED**: all four exits "ended with NOTHING on stderr". |
| `green-inbox-ack-diagnosis.log`, `green-sprint-watch-exits.log` | **GREEN**, including the pre-existing positive-lifecycle tests. |
| `agentos-full-*.log` | full `go test ./internal/agentos` green. |
| `native-lifecycle.log` | candidate binary: watch attaches and reports `conductor: s413-probe (fresh)`; `inbox-ack` against the live watch exits 0; SIGTERM produces the explained detach note; step F reproduces the yoke stale-duration defect. |

New tests: `sprint_dispatch_test.go`, `sprint_inbox_ack_test.go`,
`sprint_watch_exit_test.go`, `inbox_watcher_refusal_test.go`.

## Remaining checks (not run here)

- **Native watch soak longer than the reported ~10-minute failure window**, and
  past at least two `SprintLeaseTTL/3` heartbeats with unacked mail present. The
  bounded native probe above is ~15 s.
- `go test ./...` repo-wide, `make test`, and the Windows cross-build /
  3-OS matrix + crossvet — manager-side, per the worker brief.
- The yoke patch in §4 is **unapplied**; #1811's stale-duration acceptance
  ("report real heartbeat age or explicit unknown") cannot close until it lands.
- End-to-end `inbox-ack` under the real external-manager profile
  (`BASHY_PRINCIPAL` set, instance-UUID identity) rather than the isolated
  board used here.
