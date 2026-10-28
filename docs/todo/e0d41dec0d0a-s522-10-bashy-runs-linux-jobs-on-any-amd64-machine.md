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

Status 2026-10-28 (claude-opus5, weave #322) — PRODUCT SHIPPED, LIVE STEPS BLOCKED ON THE GUARDRAIL.

Investigated first (connect / remote install+exec / podman machine / self image / @contain / windowswslocale; there is no `bashy onboard`): the transport, the image and the CLI idiom already existed; what was missing was one verb that knows how a host ACQUIRES a Linux environment and refuses to take privilege silently. Full write-up + the live evidence: `docs/plan-linux-runner-any-amd64.md`.

Delivered: `bashy linux` (`internal/agentos/linuxrunner.go`) — `probe|plan|up|status|run`, providers `native|wsl|podman`, `--host [user@]H[:PORT]` hop, `--json`. The workspace is not copied (WSL sees the Windows path under /mnt; podman bind-mounts the cwd). One pinned distro, `Ubuntu-24.04`, so a result is comparable with noviwin1's. Default podman image is bashy's own `bashy self image` FROM-scratch build — no third-party image in the default path; a third-party `--image` must be a digest ref. `up` is idempotent and REFUSES every privileged/rebooting step (exit 2, nothing changed) unless `--allow-privileged` / `--allow-reboot`, printing the exact argv. Registered in verbSynopsis / atlas / directFrontDoorVerbs / dispatch / the e2e dispatch gate. Red/green tests in `linuxrunner_test.go`, including elephant's LIVE facts as the plan fixture and a property that the probe runs nothing mutating.

Live read-only probe of elephant (claimed as host:elephant.local:linux-runner): wsl.exe IS present — the component is absent, and `wsl --status` answers in UTF-16LE, which is why 'wsl: command not found' was misread; Windows 11 **Professional** build 26300.9711; VirtualMachinePlatform + Microsoft-Windows-Subsystem-Linux both **Disabled**, LxssManager/vmcompute absent; systeminfo reports **a hypervisor already detected**, so firmware virtualization is ON and the headless-BIOS risk is GONE; the ssh session is already **administrator**, so no UAC hop is needed.

BLOCKED: the 3 privileged steps (enable both features, then restart elephant) are posted on the sprint #522 thread as a `blocker` comment with their exact argv and the GPU/VM trade-off of turning on the hypervisor platform. Waiting on the operator. noviwin1 untouched.

Also found (separate bug, NOT this story): on elephant a PATH-resolved external command inside a pipeline fails — `sc query vmcompute` works, `sc query vmcompute | cat` says `sc: command not found`, while applet `whoami | cat` works; absolute paths work; `echo` markers also interleaved out of order. §2b of the plan doc. It does not affect this verb (the gate's pipelines run inside Linux, and `probeLinuxHost` pipes nothing), but it breaks any Windows probe written as `cmd | grep`.

Status 2026-10-28 (claude-opus5, weave #323) — **ACCEPTANCE MET on elephant.** Operator approved steps 1-7 incl. the reboot (s522-manager note, sprint #522, 18:07). Host claimed as `host:elephant.local:linux-runner` throughout; **noviwin1 untouched**.

The gate, from Dragon, one command, on a synced clean `origin/dev` worktree of cloudbox (60b4711) — `bashy linux --host liqiang@elephant.local:2222 --sync -- env GOTOOLCHAIN=local GOFLAGS=-p=4 go test -short ./...` in `hub`: **38 packages ok, zero FAIL, exit 0**, on `Linux 6.18.54.1-microsoft-standard-WSL2 x86_64` / `go1.27.2 linux/amd64`. Matches the noviwin1 WSL baseline (same package set green; `internal/api` 36.9s vs ~35s). Re-running `up` with **no** authority flags: `wsl is already set up here; nothing to do`, exit 0.

Live bring-up: both optional features enabled, elephant restarted 18:24:34Z and answered again 18:24:40Z (`uptime` = `up 0 min`, so it really rebooted), then WSL component + Ubuntu-24.04 + Go 1.27.2.

Five defects found by actually running it, each fixed with a red/green test (details and exact transcripts in `docs/plan-linux-runner-any-amd64.md` §4-§7):

1. **The `--host` hop required the verb to already exist on the host** — elephant's PATH bashy is the outpost `v1.0.0-rc.21-dev`, so the hop died with `bashy: linux: command not found`. New `--remote-bashy CMD` (a command, split into words like `BASHY_OCI`, so an `env VAR=V /path/bashy` bootstrap needs no second flag).
2. **ssh has no argv.** It concatenates its trailing arguments and the remote LOGIN SHELL re-parses them, so an unquoted hop lost the command's own quoting: `run -- sh -lc 'uname -a; free -g'` ran `free -g` on the *Windows* host. `shellJoin` now quotes word-for-word.
3. **The inbox `wsl.exe` cannot install WSL.** With both features on and the host rebooted, `--update`, `--install`, `--install --no-launch` and `--install --no-distribution` ALL answered "The Windows Subsystem for Linux is not installed. You can install by running 'wsl.exe --install'." and exited 1 — the stub's whole argument list is `--install/--update/--status/--help` and its install path wants the Store, which an unattended session does not have. The component now comes from Microsoft's **MSI** (`msiexec /i … /quiet /norestart`), resolved through `binmgr.ResolveGitHub` against `microsoft/WSL` — the same checksum-verified path `bashy self image` uses; every asset there carries a `sha256:` digest. WSL is MIT and this is runtime download + exec, so no license propagates. **This is a fleet-wide fact, not an elephant quirk — the approved step 4 was simply wrong.**
4. **`up` was not idempotent.** It reported "wsl is set up" while `status` still said "1 step(s) left: toolchain": the step provisioned Go 1.27.2 but left no `go` on PATH, and `command -v go` is that step's own Done predicate. The step now links `$(bashy go env GOROOT)/bin/go` to `/usr/local/bin/go`, which a gate written `go test ./...` needs anyway.
5. **`/tmp` is a symlink on macOS.** git answered `/private/tmp/cb-dev` while the cwd was `/tmp/cb-dev/hub`, so the remote chdir landed off the end of the synced tree. Both sides now resolve symlinks, and a cwd outside its root is refused.

Also new: `--dir PATH` (Dragon's cwd names nothing on elephant) and `--sync` (the workspace had no way to get there). `--sync` sends, from a git worktree, only what git itself would show — tracked + untracked-not-ignored + **each submodule's own files** — so cloudbox goes as 17 MB of sources rather than the 810 MB `hub/bin` would add, and `go.work`'s `use ./external/k3s` resolves (measured: `GOWORK=off go list ./...` in hub fails, so the workspace and the submodule are load-bearing). The workspace ROOT is the whole worktree, not the cwd, because `hub/` resolves through a `go.work` two levels up. The tree is replaced, not merged, under a fixed `$HOME/.bashy/linux/workspaces/` prefix.

Reconfirmed the §2b Windows bug: `sc query … | head` and `powershell.exe … | head` report `command not found` while the same commands without a pipeline work. Filed separately; it does not affect this verb.

NOT DONE — the fleet-convergence promise is only half kept: elephant runs an **unreleased build pushed to `C:/Users/liqia/.bashy/linux/bin/`** and named with `--remote-bashy`; elephant's canonical outpost bashy was deliberately NOT replaced. Once this ships in a release and elephant's bashy carries the verb, `--remote-bashy` is bootstrap-only and `BASHY_SCRATCH_BIN` (a stand-in for the release artifact this unstamped build cannot name) goes away. Remaining next steps in the plan doc §8.
