---
id: 33811caa1d10
kind: task
title: 'P0: eliminate full-timeline rescans while sprint watcher awaits inbox acknowledgment'
seq: 247
status: done
priority: p0
created: 2026-09-07T20:59:33.961542Z
sprint: 138
---

Urgent user-requested P0 resource incident, investigated 2026-09-07 on Dragon. Fix and deliver promptly; this card does not claim implementation has started.

CONFIRMED EVIDENCE
PID 50310, launched by Codex PID 48760 in the dhnt umbrella, runs: bashy sprint start 117 --owner sprint117-manager --for 4h --watch. Observed 50-75% of one CPU core, ~30 CPU minutes over ~3 hours, ~150 MB sampled footprint. The separate schedule daemon PID 72796 was below 1%. Installed bashy.real identifies revision bc9e466254567e2c4d0a97d781d843964fae2126, built with Go 1.26.5. A five-second macOS sample, symbolized using the executable's Go pclntab, shows runSprintInboxWatch -> latestSprintWatchAck -> room.Timeline -> encoding/json.Unmarshal, with JSON scanning/decoding and GC dominating sampled active work. Samples available locally at /tmp/bashy-50310-sample.txt and /tmp/bashy-50310-symbolized.txt; this written evidence must survive those temporary files.

ROOT CAUSE
bashy/internal/agentos/sprint_watch.go:166 calls ackSeq every pending-message loop; latestSprintWatchAck at :229 calls room.Timeline(0). coreutils/pkg/room/room.go:497 reads and unmarshals the whole timeline. ~/.bashy/room/timeline.jsonl measured 20,734,369 bytes / 191,575 events, including 94,100 joins and 93,685 leaves (~98% of events); history dates back to August 3. bashy/internal/agentos/inbox_poll.go sets a 100 ms minimum / 1 s maximum poll. The pending branch bypasses the normal fingerprint gate and interval backoff occurs only when pending is nil. Filesystem wakeups can shorten the wait further. Latest observed Sprint 117 ack was 2026-09-07T20:47:28Z; sampled work establishes that the watcher was waiting for another acknowledgment. Check related story a93e0023c1ee for acknowledgment design context, without expanding this incident into a redesign.

REQUIRED CHANGE
Make acknowledgment lookup incremental and avoid decoding unchanged history. Keep a safe watermark/cache/index as appropriate; handle append, partial writes, rotation, truncation/replacement and restart without losing acknowledgments or accepting another owner/sprint's ack. Bound work during unrelated filesystem churn and prolonged pending mail. Preserve explicit acknowledgment, durable unread mail, reminder cadence, cancellation, and owner-checked heartbeat/release semantics. Do not auto-ack real mail, delete history, or kill unrelated managers as a workaround. Increasing the poll delay alone does not fix the history-size-dependent cost.

ACCEPTANCE AND DELIVERY
1. Focused behavioral/performance regressions fail on the current implementation and pass with the fix. Cover unchanged history, incremental append, unrelated events, exact owner/sprint filtering, rotation/truncation/replacement, partial append, restart, prolonged pending mail, reminder cadence, cancellation and lease behavior.
2. Use hermetic synthetic histories at least 200,000 events / 20 MB. Compare smaller/larger histories and same-host before/after CPU, allocations and bytes decoded over at least 60 seconds after initial load. Require >=90% steady-state CPU reduction, zero repeated decoding for unchanged history, and appended-data-bounded work. Include unrelated filesystem activity so wakeups cannot recreate the hot loop. Record commands and actual results.
3. Audit polling and reminder output for downstream token waste. CPU waste is proven; additional inference/token consumption is NOT measured. Demonstrate polling itself makes no inference requests and report reminder frequency/output volume and any measured downstream token impact without inventing savings.
4. Run focused Bashy inbox/sprint tests and affected coreutils room tests if changed. Build/install the fix through the repo workflow, record the installed revision, and verify runtime CPU on a safely restarted watcher. Existing processes retain the old executable after installation; coordinate replacement with the active manager and preserve ownership and unread mail. Do not close based only on source tests or a replaced file on disk.

Scope: Bashy agentos watcher and, if needed, coreutils room reader; one linked story owns this incident end to end. No production log contents or personal paths should be embedded into implementation or synthetic fixtures.

DELIVERED, 2026-09-08
Coreutils PR #9 and Bashy PR #11 are merged. The incremental reader is
876d9787; the installed Bashy image is 04aff62, including graceful SIGINT/SIGTERM
cleanup. All Linux/macOS/Windows CI and the Bash 5.3 fixture gate passed.
The clean 200,000-event / 48.8 MB, 60-second comparison reduced CPU from
68.67 seconds to 4.75 seconds (93.1%) with zero unchanged-history reads or
decodes. The small-history control did not improve; no universal speedup is
claimed. Installed synthetic steady-state CPU measured 0.78% of one core;
the coordinated active watcher replacement measured 3.28% over 60 seconds.
Unread mail and owner-checked graceful lease release passed. Production
three-minute reminder output measured 226 bytes / 67 exact offline BPE tokens
under both o200k_base and cl100k_base. Polling has no inference call path;
these text counts do not establish billed usage or savings. Independent owner
verification checked nine evidence classes, source/log hashes, dependency
pins, native CI and installed-image identity before recording delivery.

FOLLOW-UP OBSERVATIONS, 2026-09-07
PID 50310 later measured 96.3% CPU at 3h08m56s elapsed. User explicitly requested direct coordination with Sprint 117 manager; status/acknowledgment and safe replacement request sent as MB post 555 by cpu-incident-investigator and repeated in Sprint 117 Meet room 10.
A newly registered investigator inbox watcher also emitted 206 lines of historical board messages (~38,419 tool-estimated output tokens before truncation), including August messages, on initial attachment. This is concrete tool-output/context-volume evidence from the investigation, not proof of extra inference calls or Sprint 117 token usage. Audit backlog delivery behavior separately from the confirmed CPU hot loop; preserve unread-message semantics and avoid silently discarding history.
