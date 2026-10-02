---
id: 91cc382c185d
kind: task
title: Build and audit one-file Darwin Bashy releases on native macOS
seq: 391
status: todo
priority: p0
labels:
    - release
    - darwin
created: 2026-10-02T21:18:40.315977Z
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
---

Current GoReleaser Ubuntu cross-build uses CGO_ENABLED=0 for Darwin, so the Sprint 355 inherited-signal gate rejects it before publish. Add a native macOS amd64/arm64 release lane with CGO_ENABLED=1, -w retained constructor symbol, the same build stamp and signed artifact manifest, a runtime inherited-ignore probe, and publication only after the audit. Preserve one physical bashy executable; no .real launcher. Integrate native outputs into the existing checksums/promotion workflow and verify both architecture assets, provenance, and failure cleanup. Until this lands, scope release claims to validated Linux/Windows targets and fail closed for Darwin.
