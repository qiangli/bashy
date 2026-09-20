---
id: 3f2dabf6ad3d
kind: test
title: 'S227.3 The airgap gate + supported-command matrix: smoke-airgap-container proves each row of docs/airgap-image.md under --network=none'
seq: 307
status: todo
priority: p1
labels:
    - airgap
    - docs
    - gate
created: 2026-09-20T21:23:20.154291Z
sprint: 227
---

"All supported commands" needs a list, and the list must be proven, not asserted. Deliver `scripts/airgap-container-smoke.sh` (Makefile `smoke-airgap-container`, dag `smoke-airgap`) modeled on `scripts/quickstart-container-smoke.sh`: builds the S227.1 image (with the S227.2 externals the matrix names), then runs one probe per row with `--network=none --read-only --cap-drop=ALL`.

Matrix (docs/airgap-image.md), three columns — builtin / pre-seeded external / not available in the image, each with the probe command and result:
- shell: bash 5.3 surface, `--posix`, `--bashsharp` (.bsh with islands, agentic{}, contracts);
- userland: the coreutils inventory (`bashy commands` output is the row source — do not hand-copy);
- git (real git is provisioned, not builtin — say which column it lands in per OS);
- dag, weave, check, transpile;
- externals per manifest;
- explicitly NOT available: podman/ollama engines (cgo, `bashy_engines`, unix host only), otel stack (`bashy_obs`), login/tessaro/sphere/cloud CLIs (remote by design — philosophy.md §3), `meet` SPA if S227.4 drops it.
Verbs whose absence must degrade with a stated limit (ast/graph without grammar blobs — see docs/TODO.md grammar note) get a row saying so.

Acceptance: the script SKIPs (exit 0) without a usable engine and FAILs on any row mismatch with an engine present; the doc's table is regenerated from the script's output (one source); one line in docs/INDEX.md + abstract in the umbrella README per the docs rule.
