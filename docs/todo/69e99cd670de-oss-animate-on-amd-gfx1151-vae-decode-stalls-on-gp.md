---
id: 69e99cd670de
kind: spike
title: 'oss animate on AMD gfx1151: VAE decode stalls on GPU — try CPU or tiled VAE decode'
seq: 3228
status: todo
priority: p2
labels:
    - media
    - oss
    - amd
    - windows
created: 2026-11-13T10:48:38.644321Z
---

Finding (Sprint 627 M3 + M3b, recorded blockers): Wan2.2 TI2V-5B on the Windows test host's AMD Radeon 8060S (gfx1151, ROCm PyTorch) completes denoise (WSL 27.7 s / native Windows 73 s) but then stalls in the GPU VAE decode and produces no MP4 (WSL 900 s timeout at 26.7/27 GiB, no OOM; native 600.72 s timeout). So the cause is the VAE decode on ROCm, not the WSL memory cap.
Experiment (cheap, denoise already works): (1) run the VAE decode on the CPU (128 GB host RAM) — move only the VAE to cpu/float32 after denoise; (2) or decode in tiles/slices on the GPU (tiled/sliced VAE decode, smaller temporal chunks); (3) check whether a ROCm/PyTorch version or an env flag (e.g. disabling MIOpen conv autotuning / a specific attention or conv backend) unblocks the decode. Windows native first (bashy.exe + .bsh rod), weights already staged and checksummed on the host (no re-download); claim the host; nothing heavy on the dev box.
Done when: one clip renders with wall time per stage and peak memory recorded, or the decode blocker is named with its root cause. If a clip renders, register the animate route=oss rod; until then drafts use the labelled still+pan fallback.
