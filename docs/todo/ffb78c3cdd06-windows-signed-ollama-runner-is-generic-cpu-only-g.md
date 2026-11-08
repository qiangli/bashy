---
id: ffb78c3cdd06
kind: bug
title: 'Windows signed Ollama runner is generic-CPU only: gpt-oss:20b at 1.2 tok/s on a 16-core Zen5 + Radeon 8060S host'
seq: 3182
status: todo
priority: p1
labels:
    - ollama
created: 2026-11-08T01:12:14.066987Z
---

Measured 2026-11-07 on the Windows test host (AMD Ryzen AI Max+ 395, Radeon 8060S, 64 GB): engines build (-tags bashy_engines,embed_runner, CGO_ENABLED=0) of bashy main with the signed windows/amd64 runner (bashy-v0.21.2-15). gpt-oss:20b (13.8 GB) ran 300 tokens in 247 s = 1.2 tok/s, prompt 2 tok/s. engine.log: inference compute library=cpu, offloaded 0/25 layers to GPU, NumThreads 16. The runner dir holds one static ycode-runner.exe (38 MB) with no ggml-cpu variant backends (upstream ships x64/sse42/haswell/skylakex/icelake... and picks the best at load) and no GPU backend (Vulkan/ROCm for AMD; CUDA is handled separately in S451.4). Expected on this host: >=20 tok/s CPU with AVX2/AVX-512 variants, far more with Vulkan on the iGPU. Fix in the runner recipe (builds/ollama): ship the CPU variant set and a Vulkan backend for windows/amd64 (and linux), keep sigstore + SBOM + license audit.
