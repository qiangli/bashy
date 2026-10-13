---
id: ad6b70841ac3
kind: bug
title: 'Expose Linux Prima no-egress failures without weakening the gate'
seq: 1908
status: done
priority: p1
labels:
    - prima
    - builds
    - no-egress
created: 2026-10-13T14:28:00Z
sprint: 436
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
---

Follow-up to story `e863c096ad55`. Candidate -2 run 38411861522 compiled
all four platforms but all no-egress gates failed; the Mac leg is repaired
(`c326ac82`, merged). The Linux legs failed with no cause in the hosted
logs: `no-egress-test.sh` redirects the whole unshare/strace/exercise block
into `out/$asset-egress.txt`, and the step exit skips the artifact upload,
so the failure cause exists nowhere the operator can see.

Investigation (code-reading; hosted logs unpullable from this worker):
a verdict failure would have printed to the step log (the verdict is not
redirected), so the Linux failure lies inside the redirected region — an
unshare/namespace-setup failure (`unshare`, `ip link set lo up`, `strace`,
`setpriv`) or an exercise failure (new binary, ring test, or synthetic
model). The -2 Linux branch is identical to the -1 branch, so this is a
blind spot in the harness, not a -2 regression in the gate logic. Leading
exercise-side suspects for the new candidate: the two-rank `--world/--rank`
flags or the `eval time` stderr assertion against the freshly built
llama-cli, and the `$(pwd)/build/bin/prima-ring-test` path. The new
diagnostics distinguish the two classes (trace present vs absent) so the
-3 run will name its cause.

Change (this worker, `builds/prima.cpp` unless noted):

1. `files/no-egress-test.sh` Linux branch: the namespace exercise keeps its
   redirect into `out/$asset-egress.txt`, but every failure path reprints
   that file to stderr before exiting non-zero — exercise failure (with a
   trace-present/absent classification hint), missing trace, and verdict
   failure. Inner phase markers (`namespace entered`, `tracing exercise`,
   `exercise finished`) go into the evidence file. Verdict invocation
   (`egress_verdict "$work/trace"` with the six ring ports) and fail-closed
   exits are unchanged; Darwin branch untouched.
2. `builds/prima.cpp/egress-evidence.yml` (new) + one `@@INCLUDE
   egress-evidence@@` line in `builds/_shared/bashy-build.yml`: the rendered
   workflow uploads `out/*-egress.txt` with `if: always()`
   (`if-no-files-found: warn`). Tools without the fragment render
   byte-identically (verified for all ten other tools). Unsigned build
   outputs still upload on success only; `release` still `needs: [pin,
   build, audit]`, so nothing signs on gate failure.
3. `builds/prima.cpp/check_test.py`: 8 new tests (Linux reprint paths,
   setup/exercise classification, verdict invocation unchanged, fork
   reproduces files, evidence upload `if: always()`, fragment content,
   unsigned-upload success-only, release gate intact). New tests fail 3/4
   against the pre-fix script (4th is the fork-identity check, green on both
   sides by construction).
4. `builds/prima.cpp/README.md`, `builds/README.md`: one paragraph/sentence
   each recording the fail-safe evidence behavior and the optional fragment.

Verified: `bash builds/check.sh` — all 11 tools reproduce, shared verdict
11 cases, Go toolchain gates 19 cases, shellcheck and actionlint clean on
the rendered fork; `python3 -B builds/prima.cpp/check_test.py` — 19 tests
pass; `bash -n`, explicit shellcheck/actionlint on the edited artifacts.
No Linux runner here, so the namespace path itself is statically verified
only; the -3 hosted run is the live proof.

Handoff: manager installs the generated `fork/` diff on the source
repository's `bashy-build` branch and cuts immutable candidate -3. No push
or Dragon native build/container from this worker. Worker stories in this
commit; manager must not commit worker stories.
