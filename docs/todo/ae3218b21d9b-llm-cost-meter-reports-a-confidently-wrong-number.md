---
id: ae3218b21d9b
kind: task
title: LLM cost meter reports a confidently wrong number (harness_estimate output tokens)
seq: 217
status: todo
priority: p2
created: 2026-09-05T17:51:03.209399Z
sprint: 126
---

FOUND while mining this host OTel spool for sprint facts (2026-09-05). Not MVP for sprint 126 — filed, not fixed.

MEASURED, ~/.agents/otel/spool/spans.jsonl, 46 gen_ai spans:
  total claimed cost  $27,492.38 across 46 calls
  tokens              49,930 in / 1,479,290 out  (out/in 31x, 24x, 17x by model)
  worst single span   11 input tokens -> 284,255 output tokens -> $5,116.79
  usage source        harness_estimate on ALL 46
  pricing_known       true on ALL 46 (0% unknown)

No model emits 284k output tokens from 11 input tokens. The output-token figure
is wrong — it is plausibly counting bytes, characters, or the whole transcript
rather than completion tokens — and cost is DERIVED from it, so the meter is
wrong by orders of magnitude.

The severity is not the number, it is the CONFIDENCE: bashy_gen_ai_pricing_known
is true on every one of these spans, so the surface asserts a known price for a
figure it cannot support. That is a success state reached without evidence, and
an operator reading a $27k total would act on it.

WHY IT MATTERS BEYOND ACCOUNTING: `sprint take --help` now tells a manager to
weigh cost when routing stories. Recorded per-call cost is not usable for that
today. The guidance was amended in the same change to route on billing MODE
(flat vs metered, from Model.Billing) and on `weave fleet` availability, which
are reliable, rather than on the meter.

FIX SHAPE (not started): establish what the harness estimate is actually
counting; either convert it to completion tokens or stop labelling the derived
price as known. `bashy_gen_ai_usage_source` already distinguishes an estimate
from metered truth — pricing_known should follow it rather than contradict it. A
red test should pin one span with a known-good in/out pair before any repricing.
