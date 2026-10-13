---
id: 5c4432c49c71
kind: test
title: Capture and review compiled C/C++/Metal inputs for official runner
seq: 1922
status: todo
priority: p1
labels:
    - sphere
created: 2026-10-13T17:09:08.458532Z
weave: 99
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
---

Audit worker b9a34015 adds Go license, native linked-library, vulnerability and SBOM scaffolding, but does not yet enumerate the actual compiled C/C++/Metal translation units and headers or map each to a reviewed permissive license; all-tracked-source hashes are insufficient. After sequentially merging b9a34015 with runtime worker df0cc6, add compiler-input capture for each of five native targets (cgo compiler invocation/dependency files and generated Metal), exact hash/license review with fail-closed unknown and explicit notices. Reuse the precise reviewed-exception pattern in builds/prima.cpp/files/license-review.py where applicable; avoid broad source-license inference and keep audit failure on unresolved inputs. Focused adversarial tests for unknown/copyleft/generated source. No Dragon native build/full suite. Worker commits with trailers, no push.
