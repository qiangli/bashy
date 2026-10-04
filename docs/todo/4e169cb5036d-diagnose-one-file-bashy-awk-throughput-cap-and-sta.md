---
id: 4e169cb5036d
kind: bug
title: Diagnose one-file Bashy awk throughput cap and startup cost
seq: 390
status: done
priority: p0
labels:
    - posix-cert
    - performance
created: 2026-10-02T23:21:34.232982Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
closed: 2026-10-04T07:13:47.959453Z
closed_by: codex-gpt6-sol
---

First one-file D diagnostic: shell set 493 TPs took 1299s versus frozen 966s; second awk set hit unchanged 600s cap versus frozen 353s, runner rc3 and no scored awk result. Measure exact one-file sh/awk cold-start versus prior lean sh and separate Coreutils on representative suite-free commands; identify cause before patch. Preserve one physical Bashy executable, POSIX semantics, inherited-signal symbol gates, and unchanged certification timeouts. Prove focused startup/awk improvement and regression behavior before any full D rerun.

## Initial triage, corrected by focused replay 2026-10-03

The sealed one-file awk journal (private SHA-256 `326219d69d6de6630af2cc740e7cd69316f6ee0abfa2409473e5e6eabe7bbe78`) has a build TP PASS at 23:09:23 UTC, execution TC start at 23:09:24, and no execution TP records before TCC SIGTERM at 23:19:24. The set returned 124 after 602 seconds. **Correction:** the journal buffered execution records; it did not prove a pre-first-TP hang. A subsequently authorized focused replay on the exact one-file candidate reached TP438/IC561 after 584 seconds, with 437 sampled PASS results in live `tet_xres`. The separate-payload reference completed 543 TPs in about 347 seconds. This is a throughput regression under the unchanged 600-second bound. Preserve licensed journal and source privately.

Suite-free local macOS arm64 builds from Bashy `30d1ced` with pinned sh `693f29bc`, Coreutils `fb7568cc`, cgo enabled and `-w` show 120 interleaved warm-launch medians: one-file `sh -c :` 16.415 ms vs lean `sh` 4.025 ms; one-file `awk 'BEGIN{print 1}'` 16.451 ms vs separate Coreutils `awk` 6.357 ms; 1000-row awk input 16.481 vs 6.327 ms. The one-file import graph initializes 762 packages and allocates 10.82 MiB, versus 293 packages/0.71 MiB for separate Coreutils. `modernc.org/libc/honnef.co/go/netdb` accounts for 2.7 ms and 3.49 MiB in the one-file trace. This tax may accumulate over many launches; quantify it on Linux before attributing the throughput gap to startup alone.

Next evidence: measure suite-free default one-file, nonshipping `-tags bashy_core` one-file, lean shell, and separate Coreutils sh/awk launch latency on an uncontended Linux host; then inspect focused replay process counts or a private strace summary to determine whether launch overhead can account for the roughly 237-second gap. Do not raise the 600-second bound or patch an unproved mechanism. The core probe lacks optional AgentOS/ycode behavior and must not replace the certified payload before route parity and build gates pass.

## Suite-free Linux profile, 2026-10-03

On host 605678097, after the focused arm terminated and its owner explicitly released a five-minute `/tmp` window, 120 interleaved POSIX-environment launches yielded these medians (p95 in parentheses): exact staged one-file Bashy SHA-256 `0d0bb35dfde16e7766abcb80d05fae6dc98ba5c73e0d84b0c9a4c5579f41afbb`, `sh -c :` **49.718 ms** (61.210), `awk 'BEGIN {print 1}'` **48.966 ms** (61.386); nonshipping core one-file, sh **16.637 ms** (21.308), awk **16.458 ms** (21.931); separate lean sh **4.754 ms** (5.975), separate Coreutils awk **10.704 ms** (13.304). The core profile remains one ELF. Its Linux ET_EXEC/runtime.fwdSig gate passed and its POSIX sh→awk smoke produced `3`. `GODEBUG=inittrace=1` showed full 762 packages/7.72 MiB init allocations, core 319/1.62 MiB, separate Coreutils 297/0.70 MiB. **This initial full/core comparison used different build modes:** the staged certification binary was static CGO with `bashy_cert`, while the core probe was pure Go. It therefore cannot isolate the import-graph effect.

If all ~237 extra seconds versus the separate awk set were launch overhead, the initial 38.262 ms per-launch gap would imply about 6,190 child starts. This is an unverified inference: no process-count trace from the licensed run is yet available, and the build-mode mismatch prevents a reliable completion-time projection. Core currently excludes optional AgentOS/ycode/Genie routes, and the shipping eligibility gate explicitly rejects `bashy_core`.

The first core probe lacked the Bash# Go-source loader. A narrow diagnostic-only import of `github.com/bashsharp/bashsharp/transpile` restores `front.GoSourceLoad` without importing AgentOS, ycode, filebrowser, telemetry, or sqlite. Focused core tests pass for one-file shell/kill, command CRUD and cert exclusion, the import boundary, a minimal `--source=go` program, and a `~~~go` fence with explicit Go toolchain. Full-product optional route parity remains unproved; do not ship the core tag yet.

## Matched Linux certification build comparison, 2026-10-03

On host 605686484, after the owner sealed the shell-only TCC and released a suite-free `/tmp` window, we built `CGO_ENABLED=1 go build -trimpath -tags bashy_cert,bashy_core -ldflags '-linkmode external -extldflags -static'` from the same Bashy revision `3907df3` as the staged candidate. The diagnostic core ELF SHA-256 was `d052153c506f5b7dc5a5bec4aad68979fce3ba9314f339f8d5a6796b16ff8e0a`, 46 MiB, ET_EXEC with `runtime.fwdSig`; the staged full certification ELF SHA-256 was `4f5346ec3ad43a658c4e117efdd21c8c036dca962f7631ad769adb8e07b8af83`, 166 MiB, also static CGO. Core omitted version stamps from ldflags; these do not change the import graph. The shipping eligibility gate still rejects the core tag.

With `POSIXLY_CORRECT=1`, `LANG=POSIX`, and `LC_ALL=C`, 120 warm interleaved launches of each route gave these medians (p95): full `sh -c :` **60.141 ms** (70.289), full `awk 'BEGIN{print 1}'` **58.884 ms** (71.098), static-CGO core sh **18.787 ms** (23.694), static-CGO core awk **18.727 ms** (22.645). Both sh→awk smoke checks printed `HI`. The core build is about 3.1 times faster for these launches under matched CGO/static settings. A full untagged pure-Go build from the same revision yielded sh **63.680 ms** (76.344) and awk **62.258 ms** (75.632), so turning CGO off alone did not reduce this observed startup tax; that pure-Go build cannot be a certification candidate because `bashy_cert` requires Linux CGO for the inherited-signal snapshot. These are host-specific launch measurements, not evidence that the licensed awk set now fits the unchanged 600-second cap. The remaining production task is to migrate optional behavior behind a base/core boundary with full route parity, then validate a new one-file certification candidate in a focused replay.

## Sprint 355 acceptance evidence 2026-10-04

The startup/throughput cause and matched build measurements are recorded at 82cc3c7 and 3907df3. The guarded base candidate then completed the full6 awk set and all 117 sets with zero caps under unchanged TCC limits. Historical raw journals and any pending formal certification decisions are unchanged.
