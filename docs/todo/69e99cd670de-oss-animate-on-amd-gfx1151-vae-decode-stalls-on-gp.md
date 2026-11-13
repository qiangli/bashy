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

## Upgrade applied and verified (2026-11-13, operator-approved)

- Restore point #1 created first; old driver package (u0196552.inf,
  32.0.22040.7) kept in the driver store for rollback.
- Driver: AMD's Ryzen AI Max+ 395 page listed 26.11.1 (optional) and
  26.10.1 WHQL; installed **26.10.1 WHQL** (1,026,657,048 bytes,
  Authenticode valid, AMD signer, SHA-256 54C208A9...) via extracted
  `Setup.exe -INSTALL` (exit 0, 104 s), then a controlled reboot at
  12:06Z. Active driver now 32.0.32015.7029 (2026-10-17), status OK,
  amdhip64_7.dll present. Klatch restored afterwards (podman-machine-default
  started; local 200, satellite route 401).
- ROCm: native Windows venv C:\rocm10\venv (Python 3.12, uv 0.13.8):
  torch 2.13.0+rocm10.0.0, torchvision 0.28.0+rocm10.0.0,
  rocm-sdk-device-gfx1151 10.0.0 (HIP 7.15). uv gotcha: --extra-index-url
  outranks --index-url, so use `--index <amd pytorch> --index <amd rocm>
  --default-index pypi`. Chose 10.0 over 10.1 because 10.1 needs a ROCm-only
  driver that does not list Ryzen.
- Kernel-level verification with random-weight decoder-shaped convs
  (bf16), one GPU job at a time, LiveKernelReports checked after each:
  sanity OK; conv2d at 512x512 output OK (13.2 s incl. 12.7 s first-call
  MIOpen setup); conv3d 128ch 5x256x256 OK (9.4 s); **conv2d at 1280x720
  output OK (7.9 s; last layers 0.24-0.48 s)**, the size that hung the
  GPU before the upgrade. 0 watchdog dumps since boot.
- Remaining: rerun the real FLUX.2 klein decode at 1280x720 and the Wan2.2
  TI2V-5B decode in this venv (S627 M8 / follow-up); the WSL path is still a
  separate track.
