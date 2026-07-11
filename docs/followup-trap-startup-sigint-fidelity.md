# Tracked follow-up: startup-ignored SIGINT — detection, not blanket skip

**Status:** the bash-5.3 fixture suite is **green** (`make test-bash` 83 pass /
0 fail / 3 skip; `trap1.sub` is byte-identical to real bash 5.3.15 both
foreground and backgrounded). This note records a **POSIX-fidelity debt**
introduced by the fix that made `execscript` pass, which no current fixture
catches but which matters for the VSC-PCTS certification push. It is a
follow-up, **not** a regression to revert.

## What shipped

sh `ecfd4042` ("interp: avoid listing harness-inherited SIGINT traps") added an
unconditional `if e.Name == "INT" { continue }` to `startupIgnoredSignals`
(`interp/signal_startup_{darwin,linux}.go`), so SIGINT is never inferred as
hard-ignored from the process's inherited signal disposition. Only the explicit
`BashyHardIgnoreEnv` bridge still marks INT hard-ignored.

## Why it was needed (the real bug)

Under the backgrounded, non-interactive `make test-bash` harness, bashy emitted a
spurious `trap -- '' SIGINT` line in `execscript` output that real bash does not
(`exec.right` has no such line). Pre-fix bashy was **false-positive detecting**
SIGINT as startup-ignored in that harness flow when real bash did not treat it
so. The fix removed the false positive → `execscript` passes.

## The debt (differential vs real bash 5.3.15)

The fix is a blanket skip, so it also removes the **true** positive. With SIGINT
*genuinely* `SIG_IGN` at entry to a non-interactive shell (the ordinary
`cmd &` / async-list case — POSIX makes background children ignore SIGINT):

| case (parent ignores INT; non-interactive child) | pre-fix bashy | **post-fix bashy** | real bash 5.3.15 |
|---|---|---|---|
| A: `trap -p INT` | `trap -- '' SIGINT` ✓ | *(nothing)* ✗ | `trap -- '' SIGINT` |
| B: `trap 'echo caught' INT; kill -INT $$; echo after` | exit 130 (dies) ✗ | `caught`+`after` (traps) ✗ | `after` (refused) |

- **A** is a genuine regression: `ecfd4042` stops bashy listing a truly
  startup-ignored SIGINT, diverging from real bash.
- **B** (the POSIX rule "a signal ignored on entry to a non-interactive shell
  cannot be trapped") was **already broken pre-fix** (bashy died with 130 rather
  than refusing); `ecfd4042` changed the *shape* of the divergence (now installs
  and fires the trap) but did not introduce it.

## The correct fix (when this is picked up)

Repair the **detection** rather than skip INT wholesale: distinguish a genuine
startup `SIG_IGN` for SIGINT (real bash lists it via `trap -p` **and** refuses to
trap it in a non-interactive shell) from the harness false-positive that
`ecfd4042` was papering over. Root-cause why `startupIgnoredSignals` mis-detected
INT under the backgrounded harness (inherited-disposition probe vs. what real
bash observes there), then gate on the real condition. Closing this also fixes
the long-standing **B** gap (install-refusal), which is a POSIX requirement the
cert suites are likely to exercise even though `trap1.sub` does not.

## How to verify a future fix

Reproduce the genuine condition and diff against real bash 5.3 (`/opt/homebrew/bin/bash`):

```sh
# A — listing
( trap '' INT; bin/bash -c 'trap -p INT' )        # want: trap -- '' SIGINT
# B — install-refusal
( trap '' INT; bin/bash -c "trap 'echo caught' INT; kill -INT \$\$; echo after" )   # want: after
```

Plus: `execscript` must still pass (no spurious SIGINT line under the harness),
and `trap1.sub` must stay byte-identical to real bash (fg + bg). Run the trap
suite on a dedicated host — `trap9.sub` orphans pgroup-escaping runaways that the
per-fixture timeout cannot reap (see the umbrella
`feedback_runaway_trap_fixture_orphans`).
