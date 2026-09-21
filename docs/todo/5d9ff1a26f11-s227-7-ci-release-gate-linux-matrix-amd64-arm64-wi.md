---
id: 5d9ff1a26f11
kind: test
title: 'S227.7 CI + release gate: Linux matrix (amd64/arm64) + Windows job run the airgap smoke on the exact candidate; listed with the other release gates'
seq: 311
status: todo
priority: p1
labels:
    - ci
    - release
created: 2026-09-20T21:23:20.271541Z
sprint: 227
---

Model: `.github/workflows/quickstart-scratch.yml` (the three-mode scratch gate, required on Linux). Add `airgap-image.yml`: Linux matrix amd64 + arm64 (QEMU/buildx or a native arm64 runner — record which and whether emulation is trusted for the size numbers), plus a `windows-2022`/`windows-2025` job for S227.5 (process-isolated containers are available on GitHub's Windows runners — verify, else the job is the named self-hosted host). Each job runs `smoke-airgap-container` and uploads the size table + `go version -m` SBOM line as an artifact.

Wire into the release procedure: one line in the release runbook (kb) naming the workflow as a gate at the `-dev` tag, like the tour workflow; do not create a new evidence format — the sprint's evidence record links the run ids.

Acceptance: workflow green on the candidate SHA that closes S227.1–S227.5; run ids in the evidence record; `make help` / `dag --list` show the new targets; umbrella bashy pin bumped with the Sprint/Story trailers.

Dogfooding note (operator, 2026-09-20): CI installs the candidate bashy and runs the gate through `bashy podman` (Linux: rootless, the embedded engine; Windows: podman cannot run Windows-native containers — nanoserver needs Docker in Windows-containers mode on the runner — so the Windows job is the one place a host engine is allowed; record it as the exception, not the pattern). Linux caveat to resolve, not assume: the model workflow `quickstart-scratch.yml` pins `BASHY_OCI: docker` because rootless podman on a GitHub runner "has its own failure modes"; this job must first prove rootless `bashy podman` (using the S227.8 linux blob — which makes the job the blob's CI consumer) and only fall back to the host engine with the failure recorded. Order: S227.8 precedes this story.
