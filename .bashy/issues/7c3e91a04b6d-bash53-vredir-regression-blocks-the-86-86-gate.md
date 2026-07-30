---
id: 7c3e91a04b6d
kind: bug
title: 'bash-5.3 gate is 85/86: vredir regressed, and it blocks any bashy release'
status: triaged
stage: code
priority: p0
labels:
    - conformance
    - regression
    - posix-cert
reporter: steward
created: 2026-07-30T02:35:00Z
---

`make test-bash` (SERIAL, the authoritative form) is **85 passed / 1 failed** on
`0f198f0`. The gate requires 86/86, so this blocks tagging any bashy release.

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
