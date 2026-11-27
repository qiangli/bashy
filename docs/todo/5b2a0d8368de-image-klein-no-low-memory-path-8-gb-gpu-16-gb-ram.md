---
id: 5b2a0d8368de
kind: enhancement
title: 'image-klein: no low-memory path (8 GB GPU / 16 GB RAM hosts and CPU-only cannot run it)'
seq: 3330
status: todo
priority: p3
labels:
    - media
    - film
    - oss
reporter: claude-opus5.5
created: 2026-11-27T19:49:34.223413Z
---

examples/oss-rods/image-klein.py loads FLUX.2 klein-4B in bf16 with enable_model_cpu_offload() and raises unless torch.cuda is available. Components: transformer 7.3 GB, text_encoder 7.5 GB, vae 0.16 GB. Model-level offload needs each component to fit on the GPU alone (~7.5 GB + context) and all of them in host RAM (~15 GB). So an 8 GB card (RTX 3070 class) with 16 GB RAM is expected to OOM or page heavily, and CPU-only or MPS hosts are refused outright. Verified fine on a 128 GB unified-memory AMD host (about 55-72 s per 432x768 still). Options: enable_sequential_cpu_offload behind a flag; an 8-bit or 4-bit text encoder (bitsandbytes or GGUF); a CPU fallback for drafts; or route through shared tools (Sprint 740 SGI). Decide with a measured run on an 8 GB host before building anything.
