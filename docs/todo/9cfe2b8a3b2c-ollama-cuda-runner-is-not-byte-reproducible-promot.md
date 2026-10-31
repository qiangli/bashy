---
id: 9cfe2b8a3b2c
kind: bug
title: 'ollama CUDA runner is not byte-reproducible: promote tested bytes instead of rebuilding on tag (for S500.1)'
seq: 3032
status: todo
priority: p2
labels:
    - release
created: 2026-10-31T10:42:48.945786Z
---

Found in Sprint 546 (S451.4a): two hosted bashy-build runs of qiangli/ollama from the same runner sources produced different linux-amd64-cuda runner digests: run 41175074040 runner sha256 4b4a46e09ae8fe6425cfd68cea417f4c398141f174aa5bda51f2761727f54861 vs run 41185680995 2444e74f9654ab24b6c38e821e4d7685483e057f4bb921f238d1e01eb048931d (only gate scripts changed between them). The release job rebuilds on the bashy-v* tag, so released bytes always differ from bytes proven on real hardware. Operator decision 2026-10-31: for now the release publishes as --prerelease, the hardware proof runs on the released assets, then promote; the real fix belongs to S500.1 'promote tested bytes, no rebuild on tag' (sign and publish the candidate run's verified artifacts instead of rebuilding), and/or make the CUDA build reproducible (find the varying input: build paths, timestamps, nvcc fatbin ordering). Verification: two candidate runs from the same commit yield identical runner + libggml-* digests, or the tag release ships the candidate run's digests.
