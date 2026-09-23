---
id: 2af48fa177ad
kind: test
title: Verify patched Bash 5.3 candidate on two Linux test droplets
seq: 320
status: done
priority: p1
labels:
    - linux
created: 2026-09-23T07:48:42.8866Z
assignee: codex-gpt-5.5
sprint: 257
sprint_id: 14e6cca7-6d3d-5712-b474-70aa075ad503
sprint_title: Verify final Sprint 253 Bash 5.3 candidate on Windows
closed: 2026-09-23T08:10:53.288282Z
closed_by: codex-gpt-5.5
---

Build native Linux amd64 binaries from the same source revision as the Windows candidate, run the verified GNU Bash 5.3 86-fixture harness on two leased Ubuntu 24.04 test droplets, and record exact totals without storing host identifiers or account paths in the repository.

Both existing Ubuntu 24.04 amd64 test droplets (kernel 6.8.0-124) ran Linux binaries built from the same patched source tree as the Windows and macOS candidates. The official GNU Bash 5.3 fixture archive was verified against its pinned SHA-256; seven glibc locales were provisioned. Each run used a controlling terminal and an unprivileged account. Both results: **86 listed, 86 runnable, 86 passed, 0 failed, 0 skipped, 0 timed out**. The shared Linux testee SHA-256 is `4d77350bca8a1c5d5637e055e72bba59079bb91a46524713e39f68f633b4d200`. Raw log SHA-256 values are `950481ad85d0335cc300321c93faeb3b14c976fb6a7f16b6364ad92481fdb530` and `cca5afbdb72b98723674ea10b4a2997136a9ac91ddac86da5dd12d4110b1be7f`. Raw logs and connection details remain outside the repositories.
