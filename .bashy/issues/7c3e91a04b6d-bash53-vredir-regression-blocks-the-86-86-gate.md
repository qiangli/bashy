---
id: 7c3e91a04b6d
kind: bug
resolution: invalid
title: 'NOT A BUG: vredir 85/86 was an ambient-fd artifact of the test host, not a regression'
status: closed
stage: code
priority: p0
labels:
    - conformance
    - regression
    - posix-cert
reporter: steward
created: 2026-07-30T02:35:00Z
---

**CLOSED — INVALID. There is no bashy regression. The gate is GREEN: 86/86.**

I filed this on a false diagnosis and am correcting it. The full serial suite is
**86 passed / 0 failed** on `0f198f0` on a clean host. The 85/86 I first measured
came from the *test host's environment*, not from bashy.

## What actually happened

`vredir` requires fd 42 to be **closed**: after `readonly v=42`, `exec {v}>&1`
must fail, leaving `v=42`, so `>&$v` then fails and bash reports
`$v: Bad file descriptor` — which the fixture greps for.

The remote host I ran the gate on had **fds 0–148 all open**, fd 42 among them,
because outpost's SSH exec channel leaks the daemon's entire fd table into
exec'd children. With fd 42 open, `>&$v` *succeeds*, no diagnostic is produced,
and the fixture prints `bad foo 1`.

| Host | fd 42 | Result |
|---|---|---|
| clean local shell | closed | **86/86** |
| via outpost ssh exec | **open** | 85/86 — `vredir` fails |

Deterministic on both, isolated and in the full suite. Not a flake.

The real defect is the fd leak, filed separately against outpost. This is the
same hazard class that `sh@70734afb` ("deflake the bad-fd-7 redirect test
against an ambient inherited fd") already patched for fd 7 — but the durable fix
is to stop leaking fds, not to deflake each fixture.

## Harness consequence, worth keeping

**A conformance gate must not be run through a channel that leaks fds.** Running
it via `outpost ssh` silently invalidates every fd-sensitive fixture and reports
a plausible-looking failure. Same principle as never trusting a killed or
truncated run as a scoreboard: an environment artifact is not a result.

---

Original (incorrect) report follows for the record.



## The failure

```
FAIL  vredir
      output differs from vredir.right
      first diff at line 16
      want: "bar is a function"
      got:  "bad foo 1"
```

Measured on a remote macOS/arm64 test host, serial, siblings verified at their
pinned SHAs (sh `ba8ebc9c`, coreutils `dd47dfb1`, readline `36b5a209`) — so the
run tested the intended code. `vredir` is NOT in the known-flaky set
(history/coproc flake under parallel load); this was serial and reproduced twice.

## It is a regression, not a known-pending fixture

`docs/plan-bash53-roadmap-agentic.md` lists `vredir` under a historical "P2 —
feature completeness" table (named-FD `{var}>file` semantics), but that roadmap
predates the suite reaching 86/86. Green is the established baseline, so this is
a regression from it.

## NOT caused by the coreutils pin bump — measured, not assumed

Two arms, bashy source held constant at `0f198f0`, varying only coreutils:

| coreutils | Result |
|---|---|
| `dd47dfb1` (new pin) | **85 passed, 1 failed** — vredir |
| `c53aa0e` (old pin) | **84 passed, 2 failed** — vredir + one more |

`vredir` fails on both, so the pin bump did not introduce it. The newer
coreutils in fact *fixes* a different fixture, so the bump is a net improvement
and should not be reverted.

## Prime suspect: the strict-posix work in `sh`

The diff is about function-vs-command identification, and `sh@ba8ebc9c` (already
bashy's pin before the coreutils bump) carries several commits in exactly that
area:

- `interp: identify PATH-resolved regular builtins as built-ins in posix command -V`
- `interp: gate non-special builtins behind PATH lookup in strict posix mode`
- `interp: accept special-builtin function names silently under strictPosix`
- `interp: pin released-bash-5.3 function-name gating with focused coverage`

`want: "bar is a function"` / `got: "bad foo 1"` reads exactly like a function
losing to a PATH-resolved command under the new gating.

## Next step

Bisect `sh` across those commits with the merge-gate discipline that exists for
this: cherry-pick ONE, run serial `make test-bash`, keep only if 86/86 stays
clean. Per-issue `go test` does not catch fixture regressions, which is how this
reached the pin.

Note the second failure visible in the old-coreutils arm was not named in the
captured output; re-run that arm if its identity matters.
