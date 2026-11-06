---
id: 3a3c1bb25f71
kind: bug
title: Preserve SIGTTOU disposition after fixture terminal restoration
seq: 3101
status: done
priority: p0
created: 2026-11-06T03:14:41.896664Z
assignee: codex-gpt6-sol
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
closed: 2026-11-06T08:10:44.70044Z
closed_by: codex-gpt6-sol
---

Authoritative lean cmd/bash86 gate found execscript/trap/varenv output includes an unexpected ignored SIGTTOU trap after earlier fixtures. Reviewed runner27ff terminal restoration temporarily calls signal.Ignore then Reset; pinned Go signal behavior may leave the ignored disposition for subsequent exec children. Native regression default disposition fails while explicitly inherited ignore remains required; establish exact captured red cause, native GNU5.3 same-environment controls and narrow runner-layer repair. Preserve TTY foreground handoff/restoration ownership checks, default versus intentionally inherited ignore, noTTY, cancellation and process cleanup. Do not normalize trap output, alter fixtures, change engine behavior speculatively or widen60/120s limits. Verify existing runner controls plus sequential child-disposition red/green and original3 cases, then full86 serial. Separate setup failures (native hexdump and C.utf8 provider) retain their exact classification.
