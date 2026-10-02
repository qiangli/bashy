---
id: 4e169cb5036d
kind: bug
title: Diagnose one-file Bashy awk pre-first-TP cap and startup cost
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

## Initial triage, 2026-10-02

The sealed one-file awk journal (private SHA-256 `326219d69d6de6630af2cc740e7cd69316f6ee0abfa2409473e5e6eabe7bbe78`) has a build TP PASS at 23:09:23 UTC, execution TC start at 23:09:24, then **zero execution TP starts/results** before TCC SIGTERM at 23:19:24. The set returned 124 after 602 seconds. The frozen separate-payload run began its execution TC at 09:26:58, first TP at 09:27:13 (15 seconds), then recorded 544 execution TPs and completed in 353 seconds. Thus the new cap is a pre-first-TP hang, not demonstrated cumulative per-TP startup cost. No live process snapshot survived the aborted arm. Preserve licensed journal and source privately.

Suite-free local macOS arm64 builds from Bashy `30d1ced` with pinned sh `693f29bc`, Coreutils `fb7568cc`, cgo enabled and `-w` show 120 interleaved warm-launch medians: one-file `sh -c :` 16.415 ms vs lean `sh` 4.025 ms; one-file `awk 'BEGIN{print 1}'` 16.451 ms vs separate Coreutils `awk` 6.357 ms; 1000-row awk input 16.481 vs 6.327 ms. The one-file import graph initializes 762 packages and allocates 10.82 MiB, versus 293 packages/0.71 MiB for separate Coreutils. `modernc.org/libc/honnef.co/go/netdb` accounts for 2.7 ms and 3.49 MiB in the one-file trace. This tax matters for repeated launches, but cannot explain a 600-second stall before one TP.

Next evidence: capture the blocked child and wait point during a licensed **focused awk** replay already authorized by the sprint owner, preferably process tree plus `/proc/<pid>/wchan`, kernel stack and command line at 30–60 seconds after TC start. Do not raise the 600-second bound or patch an unproved mechanism. A suite-free Linux sh/awk microbenchmark may quantify the startup tax but cannot by itself close this story.
