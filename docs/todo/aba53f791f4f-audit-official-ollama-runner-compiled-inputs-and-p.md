---
id: aba53f791f4f
kind: test
title: Audit official Ollama runner compiled inputs and publish SBOM gate
seq: 1921
status: wontfix
priority: p1
labels:
    - sphere
created: 2026-10-13T16:58:28.344219Z
weave: 98
assignee: codex-gpt6-luna
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T18:42:13.824947Z
---

Build on Bashy main 0686f21f official v0.21.2 runner recipe. Replace unconditional builds/ollama/audit.yml hold with a fail-closed full compiled-input dependency/license audit, generated Metal input checks, Go/native linked library inventory, OSV/govulncheck/Grype vulnerability policy and CycloneDX SBOM plus notices across all five targets. Review permissive licenses and no unresolved reachable findings; do not waive or claim that source inventory alone is clearance. Keep changes scoped to builds/ollama/audit.yml, new audit scripts/tests, and generated fork equivalents; coordinate with concurrent runtime worker df0cc6a1965c, which owns no-egress-test.sh. Worker commits with trailers, no push. Conductor runs hosted gate/release after review; no dev-host heavy build/full suite.

Merged into Sprint 436 parent story 732cbb22d353 (signed embedded runner shipped). Preserve existing commits, fork tags, failed-run evidence and this checklist under the parent; this child is not independently accepted. Required release targets are Linux amd64/arm64, Darwin arm64, and Windows amd64; Darwin amd64 is optional. The operator restored Windows to Sprint 436 scope.
