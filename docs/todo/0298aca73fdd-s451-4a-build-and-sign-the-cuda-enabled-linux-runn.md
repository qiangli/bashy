---
id: 0298aca73fdd
kind: task
title: 'S451.4a: build and sign the CUDA-enabled Linux runner prerequisite'
seq: 2487
status: doing
priority: p1
labels:
    - gpu
    - release
    - deferred
created: 2026-10-20T17:14:54.220334Z
assignee: claude-sonnet5.5
sprint: 546
sprint_id: ae111a75-889f-50a4-a423-de1bab0fbca6
sprint_title: On-demand cloud GPU workers for large/custom Ollama models via Cloudbox and Outpost (AWS EC2 first)
---

Scoped S451.4 prerequisite from docs/sprint-451-master-execution-plan.md. Build the official pinned Ollama Linux amd64 CUDA runner in the existing Bashy verified-build pipeline; retain pristine upstream provenance, native protocol/no-egress gate, compiled-input license/vulnerability/SBOM review, per-asset Sigstore identity and digest verification. Do not clear a hold without measured evidence. First red: a recipe fixture test must fail when the CUDA target lacks its compiled backend/shared libraries or release manifest/signatures. Host: Dragon for recipe/unit only, puppy.local for local NVIDIA runtime qualification; hosted CI for native build/audit/sign. No GPU droplet without explicit operator approval. Gate: bash builds/check.sh ollama, hosted build/audit success, gh release asset/signature verification, then puppy CUDA model-load probe. Evidence: docs/sprint-451/evidence.md S451.4. The S451.4 story remains owner of GPU provisioning and final acceptance.

Operator decision 2026-10-20 (Sprint 451 card): a pinned, sha256-verified NVIDIA CUDA toolkit may be used as a build-only SDK in Bashy CI, with its exact version in the SBOM. The NVIDIA driver and CUDA runtime libraries are host-provided and dynamically loaded; never bundle, statically link, or redistribute them. Missing host driver/runtime must fail closed with an actionable message, with CPU only on explicit selection. Audit, SBOM, keyless Sigstore, and binmgr identity/digest verification still apply to our runner bytes. Prove GPU model load on puppy.local before release. The policy clarification is in docs/bashy-service-supply-chain-policy.md. This does not authorize a GPU droplet.

Manager brief (s546-manager, 2026-10-31): resume, do not restart. Prior candidate: Bashy builds/ollama at e26410f1 (CCCL nv/target red fixed) and generated fork branch bashy-build ac54807 of qiangli/ollama; hosted nonpublishing run 39478338204 was canceled mid-compile. Rebase the recipe onto current bashy main, regenerate the fork, and dispatch a NONPUBLISHING hosted build; fix each build red inside this story with a red recipe assertion first (python3 builds/ollama/test_recipe.py; bash builds/check.sh ollama). Then compiled-input audit + SBOM. Stop before any publish/release/tag and before touching puppy.local: report to s546-manager (both need operator approval). Never bundle libcuda/cudart/cuBLAS. No podman/ollama on Dragon. Evidence goes to docs/sprint-546/evidence.md via the manager.

Manager decision + brief v2 (s546-manager, 2026-10-31T01:35Z) [decided]: Hosted run 39478338204 did NOT cancel: the CUDA compile COMPLETED (libggml-cuda.so linked at 21:53:29Z) and our own gate failed: `cuda-link-audit: libcuda is linked directly; the driver must be dlopen'd from the host` (upstream ggml-cuda links CUDA::cuda_driver for its VMM pool). Decision: build with `-DGGML_CUDA_NO_VMM=ON` so the backend NEEDs only libcudart/libcublas and libcuda is reached through cudart from the host; the audit stays strict (no relaxation, no policy reinterpretation). Worker scope (no push, no dispatch — the manager pushes the fork branch and dispatches the hosted run): (1) RED first in builds/ollama/test_recipe.py: assert the CUDA cmake configure line in files/build-runner.sh carries -DGGML_CUDA_NO_VMM=ON; (2) add the flag; regenerate fork/bashy-build so `bash builds/check.sh ollama` parity passes; (3) the windows-amd64 leg of test_native_recipe_and_packaging_with_fake_compiler fails on macOS (`C:/var/...` compiler path): it passed 24/24 in Sprint 451, so find the regression and fix it inside this story (red stays red until fixed; if it is a pure host artifact, prove it with the exact cause); (4) commit with trailers and submit with `python3 builds/ollama/test_recipe.py` (24/24) + `bash builds/check.sh ollama` output. Leave KB.md alone.
