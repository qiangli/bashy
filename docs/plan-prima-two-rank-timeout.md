# Prima two-rank timeout — Sprint 436 / Story 1910

Story-ID: `ab97d2a7bc77`. No source repair is applied by this delivery;
**the hosted two-rank gate remains red and signing remains blocked**.

## Plan and delivery

Read authenticated candidate -4 job logs and egress artifacts, compare the
pinned source with the producer, preserve both-rank success requirements,
add bounded diagnostics, regenerate the fork, run the lightweight producer
gate, and commit with the requested trailers. No native compilation, engine
execution, container, full suite, push, or branch switch on dev-host.

## Evidence read on 2026-10-13

[Run 38416093564](https://github.com/qiangli/prima.cpp/actions/runs/38416093564),
artifact tag `bashy-bashy-prima-0.1.0-4`, generated-tree commit
`fa03070540bf44417f4aa62d625759ddc89c395d`, source
`f4ea9845dc91a18793921492e50ef23ec799e93e`.
Authenticated reads used `gh api .../actions/jobs/JOB/logs`, the run/artifact
APIs, `gh run download ... -p '*-egress'`, and the exact-source tarball API.

| Evidence | Job/artifact ID | Downloaded file SHA-256 |
|---|---|---|
| Linux amd64 job log | 115360893303 | `4ac9bc0ed74e5ec26bc3a3a7aa88f867dca46ecb8b4a71979f5b98d0bb87b19a` |
| Darwin arm64 job log | 115360893486 | `aae5a5b2242006b84b839d302975444dd7233902a875a8e404070e9759761559` |
| Linux amd64 egress text | 11823443651 | `caea54041a95d35fc92afce8b1d4c9c9263280ca4ec030e042d2f1bbd810bc12` |
| Darwin arm64 egress text | 11824437281 | `c971271c2df5fa19dfdd083735be30d1e7fa9c9b96f37adafb9598836a5fc070` |

Both jobs passed exact pin, native CMake, standalone nng tests and single-rank
inference. Linux artifact lines 219/321 and Darwin lines 411/610 show the two
ranks reaching warmup. Crucially, **rank 0 then generates and prints timings**:
Linux lines 333–338 (`ak`, prompt evaluation 2.79 ms), Darwin lines 624–630
(prompt evaluation 0.25 ms). The later traceback is `peer.wait(timeout=90)`
on rank 1, not a decode error. Linux job lines 723–724 confirm the exercise
failed after syscall tracing started. Darwin's positive control was denied
with EPERM. No evidence demonstrates a port allowlist or model defect.

The old harness waited rank 1 first and never reported rank 0's exit status.
Its finally block killed surviving peers, then printed unlabelled logs. The
Linux failure branch discarded its syscall trace on temporary-directory exit.
Consequently these artifacts cannot prove the exact blocked syscall or whether
rank 0 was also waiting for shutdown. Do not claim the missing trace was read.

## First-party source defect and exact repair sites

The endpoint arithmetic matches the harness: `src/llama.cpp:20509–20512`
binds data at base+rank, dials data at base+next-rank, and binds signal at
signal-base+rank. `20943` initializes original-next-rank correctly. Both
ranks reaching generation rules out a blanket startup/transport failure.

The demonstrable source defect is in
[`llama_free_sockets`, lines 20884–20913](https://github.com/qiangli/prima.cpp/blob/f4ea9845dc91a18793921492e50ef23ec799e93e/src/llama.cpp#L20884):

```cpp
signal_sender.connect(endp);  // shim uses asynchronous NNG_FLAG_NONBLOCK dial
if (my_rank == 0) {
    signal_sender.send(zmq::message_t("STOP", 4), zmq::send_flags::dontwait);
}
// ... then indefinitely receives STOP, forwarding it with another unchecked send
```

`common/zmq_nng_shim.hpp`, `socket_t::send`, returns an empty optional on
`NNG_EAGAIN` or `NNG_ETIMEDOUT`. Both STOP sends discard that result. A new
connection is not necessarily ready, so STOP can be lost and the receiver
waits forever. This is a source-level liveness defect, consistent with the
hosted post-generation stall; **its occurrence in this run is inferred, not
proven by a captured syscall trace**.

The source-owner patch must make these concrete changes before repinning:

1. In `common/zmq_nng_shim.hpp`, add a `sndtimeo` setter mapping to
   `NNG_OPT_SENDTIMEO`, parallel to the existing `rcvtimeo` setter. In
   `llama_free_sockets`, replace **both** unchecked `dontwait` sends with
   checked sends using a finite shutdown deadline (for example 5 seconds):
   set `sndtimeo`, use `send_flags::none`, require an engaged result of four
   bytes, and propagate timeout/error as a nonzero engine exit. Increasing
   the harness timeout or inserting a sleep does not repair lost messages.
2. Bound rank 0's receive after initiating STOP and validate the returned
   message equals STOP; report rank and signal endpoint on failure. Workers
   may wait for shutdown during normal generation, but forwarding and
   completion must have deadlines. Audit sender lifetime: this nng shim
   ignores linger. Keep senders alive until delivery is acknowledged; a
   successful enqueue followed by destruction is not an application-level
   delivery guarantee. Add explicit per-hop acknowledgment if required by
   the tested transport lifetime. Do not close a forwarding sender merely
   because `send()` accepted the bytes.
3. Remove the C++ data race between `examples/main/main.cpp:678` (reading
   `char * stop_signal`) and `src/llama.cpp:20910–20912` (writing/allocating
   through `char **`). Store a `std::atomic<bool>` stop flag in `llama_context`,
   publish with release after validating STOP, expose an acquire-reading
   `llama_stop_requested(ctx)` via `include/llama.h`, and use it in the worker
   loop. Read/free the legacy string only after joining the signal thread.
   Keep the existing 1000 ms metadata receive timeout (`llama_recv_meta`) so
   the loop can observe stop even without a subsequent data message.
4. Add first-party tests for delayed signal connection, failed STOP send,
   forwarding sender lifetime, missing acknowledgment, and race-free worker
   exit. Both ranks must finish with zero status on the real two-rank case;
   lost STOP/failed acknowledgment must terminate nonzero within the deadline.

These are required source edits, not a validated drop-in source patch. A
producer overlay cannot honestly certify them without the first-party review
and hosted runtime tests. No source tag/pin, `prepare.py`, or native source
is changed here. The next source tag must contain the tested repair; re-run
all four hosted lanes before approving any artifact or consumer checksum.

## Producer changes and validation

`exercise.py` labels configured data/signal endpoints (not claimed live
socket observations), PID, elapsed time, pre-cleanup status, warmup/evaluation
markers and final status for every rank. Both ranks share one 90-second
stage deadline and are polled together. Nonzero exit, timeout, missing master
evaluation or log overflow fails. Cleanup kills only owned process groups.
Ring and single-rank timeout output is also retained. Child log files have a
1 MiB hard size limit; displayed logs preserve at most 32 KiB at each end
with an explicit omission count.

Linux retains the same outbound syscall set plus shutdown, with timestamps
and durations. The shell caps trace-file growth with `ulimit -f 16384`
(16 MiB under hosted GNU Bash); overflow fails the exercise. Every exercise
failure appends a bounded trace excerpt before reprinting/uploading evidence.
The full trace still feeds the original seven-port egress verdict on success;
truncation only affects retained diagnostic output. Incoming accept/receive
addresses are deliberately not added to this outbound-only verdict.

The focused Python suite includes actual short-lived fixture subprocesses,
a mocked Linux namespace failure proving the trace reaches artifact and
stderr, timeouts, failed exits, output overflow and bounded excerpts. These
fixtures do not execute Prima or build native code. The explicit gate is:

```sh
bashy gate --command 'bash builds/check.sh && python3 -B builds/prima.cpp/check_test.py'
```

Local harness checks cannot turn candidate -4 green. Hosted rerun and the
first-party shutdown repair remain required; signing and consumer pins stay
blocked.

Validation result: explicit gate PASS; inspected underlying output confirms
all 11 fork declarations reproduce, 11 shared egress cases and 19 toolchain
cases pass, and all 31 Python tests pass. Shellcheck and actionlint were
available and ran through `builds/check.sh`. `git diff --check` passed.
