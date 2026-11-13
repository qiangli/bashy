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

## Root cause: KNOWN upstream bug (research 2026-11-13)

- Native Windows: matches ROCm/TheRock#5845 (open; gfx1151, Windows 11,
  ROCm 7.2/7.13, PyTorch 2.9.1): large Conv3d/VAE decode crashes, tile 256
  works and 512 does not, no OOM. AMD: MIOpen's "naive" conv solver runs past
  the Windows TDR (2 s default), causing LiveKernelEvent 141. Fixed by
  rocm-libraries#7949 (MIOPEN_NAIVE_DISABLE_IF_ALT=1 default), shipped in
  ROCm 7.14.0 / MIOpen 3.5.2 (2026-07-15). Our stack (PyTorch
  2.9.1+rocm7.2.1, driver 32.0.22040.7 dated 2025-12-01) predates it.
- WSL stall (no error): likely TheRock#9261 (gfx1151 silent hang from a lost
  HSA completion interrupt; HSA_ENABLE_INTERRUPT=0 fixed 16/16 on Linux).
  Unverified under WSL ROCDXG.
- Host facts: TdrDelay/TdrDdiDelay unset (defaults); WATCHDOG live-kernel
  dump 2026-11-13 02:42 local. The Kernel-Power 41 reboot followed a second
  job hitting the not-yet-recovered GPU. After any Event 141, reboot before
  retrying.

Safe order (one GPU job at a time; watch for Event 141):
1. CPU VAE decode of saved latents (zero GPU risk).
2. Update Adrenalin (>= 32.0.31019 per AMD's ComfyUI-on-Windows blog;
   >= 26.6.4 for ROCm 7.14) and install ROCm >= 7.14 Windows wheels in a
   fresh venv.
3. On the new stack: 512x512 decode, then tiled 256 at 1280x720, then
   untiled 1280x720; then Wan2.2 TI2V-5B tiled 256, then 512.
4. Only if stuck on 7.2.1: MIOPEN_FIND_MODE=2 and
   MIOPEN_DEBUG_CONV_DIRECT_NAIVE_CONV_FWD=0, tile 256.
5. TDR 60/60 only after 3-4 pass (it hides, not fixes; freezes the display;
   never TdrLevel=0).
WSL: a separate track (HSA_ENABLE_INTERRUPT=0 + tiled).
