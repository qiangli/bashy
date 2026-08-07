---
id: e03916274a90
kind: task
title: 'Lifetime context: reduce six memory stores to three, and stop the transcript loss'
seq: 4
status: todo
priority: p2
created: 2026-08-07T21:13:38.73914Z
---

Workstream (c) agentic features — LOWEST of bashy's a->b->c priority order. Must not preempt the POSIX conformance campaign (#1) or Bash++ Go support.

Design of record: ../docs/lifetime-context-design-2026-08.md (umbrella; PRIVATE — carries host measurements, do not copy §1 into this repo).

WHY NOW, despite being lowest priority: exactly one part is irreversible if deferred. Agent-CLI transcripts on the host are deleted on the vendor's own retention timer, and they are the only record carrying agent reasoning and command OUTPUT — the ExecHandler seam records argv/cwd/exit/duration and never stdout, stderr or reasoning. Every day of delay loses sessions permanently. Everything else here can wait for evidence.

THE FINDING: the problem is not context size, it is that bashy has six knowledge stores and nobody reads any of them. Measured: 61,084 dispatched commands -> exactly ONE unprompted reach for the store (already cited in internal/agentos/kbrecall.go). The 'recommend it louder' fix shipped in 8f10655 and changed nothing. Instructed is not structural.

M0 [DONE] Document. Umbrella doc + docs/README.md abstract + CLAUDE.md index line.

M1 (~1h, zero new code) GO/NO-GO. Record the retrieval baseline bashy-recall-spec.md §8 still owes, then hand-run the shipped-but-never-fired execlog.PromoteToKB over the existing execlog records and re-run script/memory-eval. GATE: if machine-written candidate pages hurt retrieval at 36 pages, extraction into kb is dead at transcript scale and the design becomes BM25 over verbatim with no distillation. Cheapest possible way to retire the whole approach.

M2 (~half a day) TWO LIVE BUGS, worth fixing regardless of this feature:
  (a) execlog + spacegraph + craft-facts ALL stopped writing on 2026-08-06 and nothing reported it. Diagnose, fix, and add a 'bashy doctor' stale-writer check.
  (b) the OTel span spool violates five clauses of its own invariant at once: 0644 not 0600, tens of MB, unbounded, unredacted argv, no retention.

M3 (~2 days) THE MIRROR, claude only. New coreutils/pkg/sessions: verbatim, 0600, day-partitioned mirror of harness transcripts taken before the vendor deletes them. Ingest through secrets.NewRedactor{ShapeMask} then redact.FromHost (pkg/weave/capture_redaction.go is the wiring precedent). Cursor, not an index — anything richer is a store wearing a view's clothes. No Export/Sync/marshaller, negative test. Report parse coverage every run: the transcript format has no published schema and churns fast, so golden-file tests would pass while the reader silently read less. Verb 'bashy sessions sync|ls|status', TOP-LEVEL not under kb (kb is in theLoop; a lifecycle verb must not depend on a proprietary vendor client being installed). Driven from the ExecHandler seam on a time check, NOT bashy schedule — schedule has never run and would die the same silent death as (a).

DEFERRED until M1 returns a number: BM25 over the verbatim mirror as a second kb ring with a byte-capped index.md; retiring execlog/spacegraph/craft as CAPTURE while keeping Template/Benign/Coverage/evidence-bar as ANALYSIS; the push channel (own schema, NOT bashy-hint-v1 which pkg/nudge already owns; own kill switch, NOT BASHY_HINTS which also gates learnEnabled; a control arm; and a pitfall push must RE-RUN the cheap probe rather than recite the note).

CONCURRENCY: the (a) cert campaign runs under another agent in sh/ and coreutils/. Never 'git add -A' in the umbrella. Never touch sh/ from this workstream. Take a claim before M2/M3; M3 touches shared pkg/atlas/atlas.go.

RISK TO DESIGN AGAINST: a transient failure (VPN down, daemon not up) becoming a permanent 'X is broken here' page, landing in the always-load index and pushed forever — including after X works again. The agent complies, succeeds, and that success is indistinguishable from the hint being correct. internal/agentos/learn.go already refuses to learn from failure for exactly this reason.
