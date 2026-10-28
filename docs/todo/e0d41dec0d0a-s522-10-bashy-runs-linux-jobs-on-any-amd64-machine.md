---
id: e0d41dec0d0a
kind: feature
title: 'S522.10 bashy runs Linux jobs on any amd64 machine: elephant (Windows, no WSL) becomes a Linux build/test peer through bashy, not a manual install'
seq: 2823
status: doing
priority: p1
labels:
    - windows
    - linux
    - peers
    - build
created: 2026-10-28T17:14:07.442178Z
assignee: claude-opus5
sprint: 522
sprint_id: 3413791d-7a40-5514-9970-99b64d1d83cf
sprint_title: 'AWS cloud integration and satellite hosting: bashy aws provider (Lightsail), budget guard, hosting research, self-hosted + hosted AWS satellites'
---

Operator 2026-10-28: 'bashy must run [on] amd machines regardless.' Found live: elephant (Windows 11, AMD, 2 TB / 128 GB, latest AMD GPU; the designated heavy-jobs host, reachable passwordless as liqiang@elephant.local:2222) cannot run Linux-only work (cloudbox hub tests need linux/k3s; release image builds need podman): 'wsl: command not found / The Windows Subsystem for Linux is not installed'. Today only noviwin1 (WSL Ubuntu) can, so heavy Linux jobs bottleneck on it. GOAL: on any amd64 host bashy can stand up and reuse a Linux execution environment and run a command in it, e.g. 'bashy <linux-runner> -- go test ./...' with the workspace synced, the way peers already run jobs via outpost ssh. Investigate first what exists (bashy oci/sandbox on Windows, bashy onboard recipes, release-preflight peer lanes, S515.9 Windows act_runner) and pick the smallest product path: e.g. a bashy recipe that installs/enables WSL2 + Ubuntu with Go 1.27.2 + podman non-interactively (reboot handling, idempotent, admin elevation via outpost), or a bashy-managed Linux VM where WSL is unavailable; GPL-free / permissive-only rules apply (docs/bashy-service-supply-chain-policy.md). No manual host steps: fixes ship in a bashy release and hosts converge via the recipe. ACCEPTANCE: from Dragon, one bashy command runs the cloudbox hub Linux gate (go test -short ./... in hub with GOTOOLCHAIN=local) on elephant and the result matches noviwin1 WSL; re-running is a no-op setup. Do not touch noviwin1.

GUARDRAIL (manager): elephant is the operator's power machine and runs other work. Before any step that installs Windows features, needs admin elevation, or REBOOTS elephant, stop and post the exact step on the sprint thread (bashy sprint comment 522) and wait; the manager confirms with the operator. Design + red/green tests + a dry-run plan against elephant first. Claim the host with `bashy claim host:elephant.local:linux-runner --intent ...` while you touch it.

CONTINUATION (manager 2026-10-28 ~10:58 PDT): OPERATOR APPROVED steps 1-7 incl. the elephant REBOOT now (see sprint thread). START: `git fetch origin s522-10-wip && git reset --hard FETCH_HEAD` (your ebd474d0 rebased = 5d21e1ee). Build bashy from this workspace (go build -o /tmp/bashy-s522-10 ./cmd/bashy; do NOT make install over the canonical bashy), then: (1) bashy linux --host liqiang@elephant.local:2222 up --allow-privileged --allow-reboot; (2) wait for elephant to come back (poll outpost ssh with a bounded loop, ~10 min max; elephant also reconnects its outpost to zoo); (3) second hop up --allow-privileged (wsl update/default 2/Ubuntu-24.04/bashy+Go 1.27.2); (4) ACCEPTANCE: from Dragon one bashy linux run --host elephant command runs the cloudbox hub Linux gate (go test -short ./... in hub, GOTOOLCHAIN=local, GOFLAGS=-p=4) on a synced tree of cloudbox origin/dev and compare with noviwin1 WSL baseline (manager has it: v0.18.43 gate passed there); (5) re-run up = no-op. Record exact commands + outputs in docs/plan-linux-runner-any-amd64.md, fix what breaks, commit and exit. Claim host:elephant.local:linux-runner while working; release after.
