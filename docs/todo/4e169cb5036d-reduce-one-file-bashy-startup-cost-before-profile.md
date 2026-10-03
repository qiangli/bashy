---
id: 4e169cb5036d
kind: bug
title: Diagnose one-file Bashy awk throughput cap and startup cost
seq: 390
status: todo
priority: p0
labels:
    - posix-cert
    - performance
created: 2026-10-02T23:21:34.232982Z
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
---

First one-file D diagnostic: shell set 493 TPs took 1299s versus frozen 966s; second awk set hit unchanged 600s cap versus frozen 353s, runner rc3 and no scored awk result. Measure exact one-file sh/awk cold-start versus prior lean sh and separate Coreutils on representative suite-free commands; identify cause before patch. Preserve one physical Bashy executable, POSIX semantics, inherited-signal symbol gates, and unchanged certification timeouts. Prove focused startup/awk improvement and regression behavior before any full D rerun.

## Initial triage, corrected by focused replay 2026-10-03

The sealed one-file awk journal (private SHA-256 `326219d69d6de6630af2cc740e7cd69316f6ee0abfa2409473e5e6eabe7bbe78`) has a build TP PASS at 23:09:23 UTC, execution TC start at 23:09:24, and no execution TP records before TCC SIGTERM at 23:19:24. The set returned 124 after 602 seconds. **Correction:** the journal buffered execution records; it did not prove a pre-first-TP hang. A subsequently authorized focused replay on the exact one-file candidate reached TP438/IC561 after 584 seconds, with 437 sampled PASS results in live `tet_xres`. The separate-payload reference completed 543 TPs in about 347 seconds. This is a throughput regression under the unchanged 600-second bound. Preserve licensed journal and source privately.

Suite-free local macOS arm64 builds from Bashy `30d1ced` with pinned sh `693f29bc`, Coreutils `fb7568cc`, cgo enabled and `-w` show 120 interleaved warm-launch medians: one-file `sh -c :` 16.415 ms vs lean `sh` 4.025 ms; one-file `awk 'BEGIN{print 1}'` 16.451 ms vs separate Coreutils `awk` 6.357 ms; 1000-row awk input 16.481 vs 6.327 ms. The one-file import graph initializes 762 packages and allocates 10.82 MiB, versus 293 packages/0.71 MiB for separate Coreutils. `modernc.org/libc/honnef.co/go/netdb` accounts for 2.7 ms and 3.49 MiB in the one-file trace. This tax may accumulate over many launches; quantify it on Linux before attributing the throughput gap to startup alone.

Next evidence: measure suite-free default one-file, nonshipping `-tags bashy_core` one-file, lean shell, and separate Coreutils sh/awk launch latency on an uncontended Linux host; then inspect focused replay process counts or a private strace summary to determine whether launch overhead can account for the roughly 237-second gap. Do not raise the 600-second bound or patch an unproved mechanism. The core probe lacks optional AgentOS/ycode behavior and must not replace the certified payload before route parity and build gates pass.

## Suite-free Linux profile, 2026-10-03

On host 605678097, after the focused arm terminated and its owner explicitly released a five-minute `/tmp` window, 120 interleaved POSIX-environment launches yielded these medians (p95 in parentheses): exact staged one-file Bashy SHA-256 `0d0bb35dfde16e7766abcb80d05fae6dc98ba5c73e0d84b0c9a4c5579f41afbb`, `sh -c :` **49.718 ms** (61.210), `awk 'BEGIN {print 1}'` **48.966 ms** (61.386); nonshipping core one-file, sh **16.637 ms** (21.308), awk **16.458 ms** (21.931); separate lean sh **4.754 ms** (5.975), separate Coreutils awk **10.704 ms** (13.304). The core profile is 66.4% faster than full Bashy for awk startup and still one ELF. Its Linux ET_EXEC/runtime.fwdSig gate passed and its POSIX sh→awk smoke produced `3`. `GODEBUG=inittrace=1` showed full 762 packages/7.72 MiB init allocations, core 319/1.62 MiB, separate Coreutils 297/0.70 MiB. The fastest safe fix is therefore a smaller base/core import graph, not a millisecond tweak to one optional initializer.

If all ~237 extra seconds versus the separate awk set are launch overhead, the measured 38.262 ms per-launch gap implies about 6,190 child starts; with the core profile's 5.754 ms gap, the same work projects around 383 seconds. This is an inference: no process-count trace from the licensed run is yet available. Core currently excludes optional AgentOS/ycode/Genie routes, and the shipping eligibility gate explicitly rejects `bashy_core`.

The first core probe lacked the Bash# Go-source loader. A narrow diagnostic-only import of `github.com/bashsharp/bashsharp/transpile` restores `front.GoSourceLoad` without importing AgentOS, ycode, filebrowser, telemetry, or sqlite. Focused core tests pass for one-file shell/kill, command CRUD and cert exclusion, the import boundary, a minimal `--source=go` program, and a `~~~go` fence with explicit Go toolchain. Full-product optional route parity remains unproved; do not ship the core tag yet.
