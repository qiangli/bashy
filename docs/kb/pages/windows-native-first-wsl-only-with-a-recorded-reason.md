---
id: 01a1d304-a339-7363-9165-0262fa2bf057
seq: 27
form: page
type: decision
title: 'Windows: go native first; WSL only with a recorded reason'
description: 'Agents default to WSL on Windows; bashy is native on Windows, so plan native (bashy.exe, .bsh, winget/uv, vendor Windows builds) first and treat a native failure as a bashy gap — WHEN planning, delegating or reviewing any work on a Windows host, or about to run wsl, change .wslconfig or wsl --shutdown (keywords: windows, wsl, native, powershell, encodedcommand, winget, uv, wslconfig, memory cap, klatch, parity, cross-os, bias)'
tags:
    - windows
    - wsl
    - native
    - cross-os
    - parity
status: candidate
evidence: '2026-11-13: Windows test host — WSL ~31 GiB cap stalled a local video model at 26.7/27 GiB while the native process path had no such cap; WSL clock steps broke JWTs (2026-11-10); NTFS 0777 broke a fixture; one-line EncodedCommand requirement observed; operator called out the WSL-by-default bias'
source:
    tool: claude
created: "2026-11-13T03:47:38Z"
---

Agents tend to treat WSL as the default way to work on Windows. Most of
what they learned assumes Linux and CUDA, and native Windows means
PowerShell quoting and different paths. bashy runs natively on Windows,
so on a Windows host the native path comes first. WSL is a fallback that
needs a recorded reason.

## Rule

1. **Plan the native route first.** Use the native `bashy.exe`, Bash#
   `.bsh` commands, tools pinned with `winget` or `uv`, and the vendor's
   Windows build of a library (for example a Windows GPU build of
   PyTorch).
2. **Use WSL only when a native path is proven missing.** Write down why
   in the story or evidence: the command tried, the error, and the missing
   piece.
3. **A native failure is a bashy gap.** File it (a todo with the failing
   command) instead of silently moving the work into WSL. Windows users
   hit the same gap.
4. **Review delegated plans for silent WSL defaults.** A worker plan that
   starts with `wsl -d ...` and gives no reason goes back for a native
   attempt.

## What WSL costs (observed on a Windows test host)

- **Memory cap.** WSL gets a fixed share of RAM (about a quarter by
  default, ~31 GiB on a 128 GB host). A local video model stalled at
  26.7 of 27 GiB. A native process is not held to that cap, and an
  integrated GPU with shared memory can use far more.
- **Raising the cap needs `.wslconfig` plus `wsl --shutdown`.** That
  restarts every distro and any container machine running in WSL, which
  takes live services down.
- **Clock steps.** Time sync inside the distro stepped the clock back
  every ~30 s, so freshly issued JWTs were "not valid yet".
- **File permissions.** Files copied from NTFS show up as 0777, which
  breaks permission-sensitive fixtures.
- **Two environments to provision and claim,** and evidence that no longer
  matches what a Windows user runs.

## Native Windows working notes

- Remote PowerShell: pass scripts as
  `powershell -NoProfile -EncodedCommand <base64 of UTF-16LE>`, and
  encode on ONE line (`base64 | tr -d '\n'`). A wrapped encoding fails
  as "command not found" fragments. A statement that errors can blank the
  whole output, so keep probes small.
- Call native executables by full path with forward slashes. Prefer
  `uv` for a pinned Python, and `winget` for system tools.
- Cross-OS `.bsh` commands keep one interface. OS-specific provisioning
  goes inside the command (`case $(uname)`/OS checks) and is not split
  into a Linux-only path.

## When WSL is the right call

- The tool has no Windows build and none is planned (record the upstream
  issue).
- The job is a Linux container gate that must match a Linux venue, for
  example a sealed Linux test image.

Even then, keep live services' container machines out of it, and never
change `.wslconfig` or shut WSL down without the operator's approval.
