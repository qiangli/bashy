---
id: 8d5cbe5a57c7
kind: task
title: Audit and sign official Ollama runner release on five platforms
seq: 1918
status: todo
priority: p1
labels:
    - sphere
created: 2026-10-13T16:46:56.215447Z
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
---

After Bashy official v0.21.2 runner recipe, run hosted native Linux amd64/arm64, Darwin amd64/arm64 and Windows amd64 CI. Verify no-egress, actual health/load/streaming/embedding/cancellation/shutdown with small licensed GGUF, complete compiled-input license and dependency audit, SBOM, transitive library inventory, deterministic assets, exact workflow Sigstore bundles and signed SHA256SUMS. Publish immutable release only after all gates pass, then independently verify artifact bytes and attestations. No Dragon heavy build/container/full suite.
