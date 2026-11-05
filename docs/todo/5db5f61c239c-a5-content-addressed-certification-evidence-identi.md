---
id: 5db5f61c239c
kind: task
title: A5 Content-addressed certification evidence identity
seq: 3063
status: done
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T14:02:34.244841Z
weave: 16
assignee: codex-gpt6-sol
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
closed: 2026-11-04T15:27:34.622912Z
closed_by: claude-opus5.5
---

Operator-approved (2026-11-04). Goal: certification evidence stays valid across commits that do not change what the certified binaries link. SCRIPTS/DOCS ONLY — no product source, go.mod, or pin change (candidate f1bcd711 is frozen). Data motivating it: of the last 200 commits, sh 44% and coreutils 56% never touch packages linked by the cert builds. Deliver: (1) extend scripts/s561-baseline-manifest.sh (or one small sibling script) to emit cert_closure for BOTH cert builds — ./cmd/bash and -tags=bashy_cert_base ./cmd/bashy — computed for the cert env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOWORK=off: the linked package list (go list -deps), and for every linked package (main module AND dependencies, incl. sh/coreutils/bashsharp/yoke) sha256 of its compiled inputs (GoFiles, CgoFiles, EmbedFiles, SFiles, etc.; tests excluded), plus toolchain version, build tags/flags; one digest per build. Hash files, not module versions, so a new sh/coreutils commit that leaves linked packages byte-identical yields the SAME digest. (2) scripts/cert-identity-check.sh <manifest.json> [rev]: recompute at rev, print SAME or CHANGED with the changed packages/files; exit 0/1. (3) docs/s561/evidence-identity.md: the rule (evidence valid while both digests and artifact hashes are unchanged; any CHANGED requires the affected lanes to rerun), plus a row in docs/s561/inventory.md. Validation to record (manager will rerun): baseline 5a4bd03f vs candidate f1bcd711 -> CHANGED (sh interp + internal/cli listed); f1bcd711 vs current main (docs-only commits) -> SAME; one historical bashy commit touching only internal/agentos vs its parent -> SAME for cmd/bash. GOWORK=off; trailers Sprint: #561, Story: <seq>, Story-ID: <id>; BASHY_AGENT = you; do not push.
