---
id: e0d41dec0d0a
kind: feature
title: 'S522.10 bashy runs Linux jobs on any amd64 machine: elephant (Windows, no WSL) becomes a Linux build/test peer through bashy, not a manual install'
seq: 2823
status: todo
priority: p1
labels:
    - windows
    - linux
    - peers
    - build
created: 2026-10-28T17:14:07.442178Z
sprint: 522
sprint_id: 3413791d-7a40-5514-9970-99b64d1d83cf
sprint_title: 'AWS cloud integration and satellite hosting: bashy aws provider (Lightsail), budget guard, hosting research, self-hosted + hosted AWS satellites'
---

Operator 2026-10-28: 'bashy must run [on] amd machines regardless.' Found live: elephant (Windows 11, AMD, 2 TB / 128 GB, latest AMD GPU; the designated heavy-jobs host, reachable passwordless as liqiang@elephant.local:2222) cannot run Linux-only work (cloudbox hub tests need linux/k3s; release image builds need podman): 'wsl: command not found / The Windows Subsystem for Linux is not installed'. Today only noviwin1 (WSL Ubuntu) can, so heavy Linux jobs bottleneck on it. GOAL: on any amd64 host bashy can stand up and reuse a Linux execution environment and run a command in it, e.g. 'bashy <linux-runner> -- go test ./...' with the workspace synced, the way peers already run jobs via outpost ssh. Investigate first what exists (bashy oci/sandbox on Windows, bashy onboard recipes, release-preflight peer lanes, S515.9 Windows act_runner) and pick the smallest product path: e.g. a bashy recipe that installs/enables WSL2 + Ubuntu with Go 1.27.2 + podman non-interactively (reboot handling, idempotent, admin elevation via outpost), or a bashy-managed Linux VM where WSL is unavailable; GPL-free / permissive-only rules apply (docs/bashy-service-supply-chain-policy.md). No manual host steps: fixes ship in a bashy release and hosts converge via the recipe. ACCEPTANCE: from Dragon, one bashy command runs the cloudbox hub Linux gate (go test -short ./... in hub with GOTOOLCHAIN=local) on elephant and the result matches noviwin1 WSL; re-running is a no-op setup. Do not touch noviwin1.

GUARDRAIL (manager): elephant is the operator's power machine and runs other work. Before any step that installs Windows features, needs admin elevation, or REBOOTS elephant, stop and post the exact step on the sprint thread (bashy sprint comment 522) and wait; the manager confirms with the operator. Design + red/green tests + a dry-run plan against elephant first. Claim the host with `bashy claim host:elephant.local:linux-runner --intent ...` while you touch it.
