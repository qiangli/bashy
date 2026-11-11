---
id: 01a1b809-644d-74ba-a46a-27a89863f47d
seq: 15
form: page
type: gotcha
title: 'Windows remote ops over outpost ssh: full exe paths, -EncodedCommand, --stdin elevation'
description: 'How to run commands on a Windows host through outpost ssh without losing output or quoting — WHEN operating a remote Windows host via outpost (keywords: windows, outpost ssh, powershell, EncodedCommand, PATH, forward slashes, outpost connect --stdin, scp, lost output)'
tags:
    - windows
    - outpost
    - ssh
    - powershell
scope:
    os: windows
status: candidate
evidence: '2026-11-07: used repeatedly against a native Windows host during multi-satellite work; full-path PowerShell form also in outpost/docs/pairing.md'
source:
    tool: claude
created: "2026-11-07T22:03:05Z"
---

## Facts

- `outpost ssh` to a Windows host runs Bashy's in-process shell with a minimal PATH, not cmd.exe. Bare names like `tasklist` fail; run native executables by full path with FORWARD slashes, e.g. `C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe`.
- Pass PowerShell as `-EncodedCommand` (base64 of UTF-16LE) so no quoting layer mangles it:

```sh
PS_SCRIPT='Get-Service | Select-Object -First 5 | Out-String'
ENC=$(printf %s "$PS_SCRIPT" | iconv -f UTF-8 -t UTF-16LE | base64 | tr -d '\n')
outpost ssh "$HOST_NAME" -- C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe -NoProfile -EncodedCommand "$ENC"
```

- A statement that errors can lose ALL of the command's output. Keep each remote command small, and guard statements (`try { } catch { $_ | Out-String }`, `-ErrorAction SilentlyContinue`) so you still see what ran.
- Elevate non-interactively from a vault-held password (never echo it):

```sh
bashy secret exec "$HOST_PW_SECRET=P" -- /bin/sh -c 'printf %s "$P" | outpost connect "$1" --stdin' sh "$HOST_NAME"
```

- Copy files with `outpost scp`. `--safe` applies to uploads only; omit it for downloads. Remote paths use forward slashes; on the Windows side prefer `$env:USERPROFILE` / `%LOCALAPPDATA%` over spelled-out user directories.
- Multi-stage pipes inside the remote Bashy shell can break on Windows; prefer one PowerShell invocation that does the whole job.
