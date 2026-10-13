---
id: 67ee067347a4
kind: bug
title: 'Repair Prima hosted no-egress loopback exercise'
seq: 1909
status: done
priority: p1
labels:
    - prima
    - builds
    - no-egress
created: 2026-10-13T15:00:00Z
sprint: 436
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
---

Follow-up to stories `e863c096ad55` (Mac direct-EPERM control) and
`ad6b70841ac3` (Linux fail-safe evidence). Candidate -3 run 38414127942
(commit `d77e37ef`, `bashy-build` branch of `qiangli/prima.cpp`) compiled
all four native targets but failed all four no-egress gates, so audit and
release SKIPPED: pin gate passed, each `native CMake build` step passed,
each `no-egress gate (before signing)` step failed, and the new
`if: always()` evidence upload succeeded on all four legs (per-leg
`-egress` artifacts exist and are unexpired).

Investigation (public GitHub API for job/step verdicts plus pinned-source
reads; hosted step logs and egress artifacts are auth-gated from this
worker, so log lines below are inferred, not quoted):

1. Ring-test source at the pinned commit (`f4ea9845`, file created once in
   `ca4b56ad`, unchanged since) binds/connects THREE loopback ports, read
   in full via raw fetch: `tcp://127.0.0.1:5599` (3-part multipart),
   `tcp://127.0.0.1:5600` (rcvtimeo, no sender), `tcp://127.0.0.1:5601`
   (single-message send/recv). Both of our allowlists carried only
   5599/5600 plus the engine ports — 5601 was missing from day one.
2. Mac legs: the sandbox profile denied the 5601 connect (EPERM). The
   shim dials with `NNG_FLAG_NONBLOCK`, so the denied dial reports no
   synchronous error, the non-blocking send buffers, and test 3's blocking
   `recv` hangs until `exercise.py`'s 30s `subprocess.run` timeout fires —
   the reported "Mac ring test times out". Corroboration: -2 Mac legs
   failed in the positive control before the engine ever ran, so -3 is the
   first run to reach the ring test under the sandbox; the 616/617-byte
   Darwin egress artifacts fit tests [1]+[2] passing plus a timeout
   traceback, and the -3 failure moved past the (passing) EPERM control.
3. Linux legs: the namespace does not filter loopback, so the ring test
   itself passes there, but the strace verdict would flag the 5601
   loopback connect as egress — the same omission, fail-closed in the
   wrong direction. The reported "Linux rank 1 times out" is the two-rank
   phase: `peers[0]` (rank 1) `wait(timeout=90)` expired. The engine maps
   `port = data/signal_port + rank`, so the two-rank ports (19000/19001,
   19100/19101) were already allowed; the startup/gather/bcast barrier
   sequence for an explicit `1,1` window was read end to end
   (`common.cpp` init order, `llama_init_sockets` binds own / dials next,
   no `rebuild_topo` on the explicit-window path) and shows no
   statically visible deterministic deadlock, and the millisecond-scale
   synthetic model (2 blocks, dim 16) rules out honest slowness tripping
   a 90s deadline. The exact rank-1 hang point is therefore NOT proven
   from reachable evidence — recorded below as remaining uncertainty,
   not fixed by invention.
4. Out of scope by evidence: engine internals (pinned first-party
   source), timeout tuning, launch order — no OUR-side defect found there.

Change (this worker, `builds/prima.cpp` unless noted):

1. `files/no-egress-test.sh`: Linux verdict call becomes
   `egress_verdict "$work/trace" 5599 5600 5601 19000 19001 19100 19101`
   with a comment citing the three ring-test ports and the -3 run; the
   Darwin sandbox `allow network-outbound` line gains
   `(remote ip "localhost:5601")`. Deny rule, positive control, engine
   EPERM tripwire, and unified-log sweep are byte-identical otherwise.
2. `fork/bashy-build/no-egress-test.sh`: regenerated with
   `bash builds/render.sh prima.cpp` (generator output, not a hand edit).
3. `check_test.py`: Linux/Darwin port assertions updated to the
   seven-port set; new `NoEgressRingPort5601` class pins the exact
   verdict argv and the exact profile allow-set, plus a
   loopback-only guard on the Darwin profile. Fail-closed tests
   (non-loopback denial, EPERM control, engine tripwire, verdict
   strictness via the shared 11-case suite) untouched.
4. `README.md`: documents the exact seven-port allowlist with per-port
   roles and the -3 history.

Verified: `python3 -B builds/prima.cpp/check_test.py` — 22 pass (19
prior + 3 new); the 2 membership tests fail against the pre-fix script
(authentic failure checked via stash) while the loopback-only guard
passes on both; `bash builds/check.sh prima.cpp` — fork reproduces,
shared verdict 11 cases and Go toolchain gates 19 cases hold;
`bash -n` on both scripts; `shellcheck -S warning` and `actionlint`
clean. No CMake build, engine run, container, or full suite from this
worker (per directive); no Linux runner here, so the namespace path is
statically verified only.

Remaining CI uncertainty (for the -4 candidate): (a) the Linux rank-1
hang root cause is unproven — if it persists with 5601 fixed, the -3
reprint-to-stderr plus the `if: always()` egress upload will carry the
rank logs and traceback, which this worker could not read; (b) Mac
recovery assumes the hang was the denied 5601 connect (strong but
log-unconfirmed); (c) no hosted proof exists yet that the seven-port
gate passes on any leg. Nothing was weakened: any non-loopback attempt
still fails the verdict/sandbox, and the EPERM positive control still
gates the Darwin legs.

Handoff: manager installs the generated `fork/` diff on the source
repository's `bashy-build` branch and cuts immutable candidate -4. No
push or Dragon native build/container from this worker.
