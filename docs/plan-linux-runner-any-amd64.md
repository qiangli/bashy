# `bashy linux` — a Linux execution environment on any amd64 host

Sprint: #522 · Story: #2823 · Story-ID: e0d41dec0d0a · 2026-10-28

Operator (2026-10-28): *"bashy must run \[on] amd machines regardless."*

Linux-only work — the cloudbox hub gate (linux/k3s), release image builds
(podman) — bottlenecks on the single Windows host that happens to have WSL
installed **by hand** (noviwin1). elephant, the designated heavy-jobs host
(Windows 11, AMD, 2 TB, the latest AMD GPU), cannot take any of it. "Install it
by hand" does not converge a fleet, so the knowledge of how a host gets a Linux
environment belongs in the release and every host converges by running the same
verb.

---

## 1. What already existed (investigated before designing)

| surface | what it does | why it is not the answer |
|---|---|---|
| `bashy connect <host>` (`internal/agentos/connect.go`) | interactive shell on a peer through outpost ssh | interactive only; its own help says TTY commands and WSL selection "require an outpost SSH extension". No Linux provisioning. |
| `bashy remote install` / `remote exec` (`remote.go`, `remote_exec.go`) | pushes a version-matched bashy to a peer, then runs one-shot commands over an in-process SSH peer channel (port 2223) | the right *transport* shape, but it runs commands in the peer's **native** OS. On elephant that is Windows. |
| `bashy oci` / `podman` / `dks` (`engines_windows.go`, `podman_machine.go`) | podman machine, incl. a rootful k3s machine | on Windows podman machine runs **on WSL or Hyper-V**. On a host with neither enabled it fails for exactly the same reason. It is a consumer of this story, not a solution to it. |
| `bashy self image` (`self_image.go`) | fetches the checksum-verified `bashy-scratch-linux-<arch>` release artifact and builds `localhost/bashy:<ver>-linux-<arch>` FROM scratch | reusable, and the reason the podman provider needs **no third-party image**. |
| `@contain(image:)` (`contain_image.go`) | one contained call against a digest-pinned image | the policy precedent followed here: a third-party ref must be a digest. |
| `bashy limit`, `bashy awd` | wrapper verbs with hand-rolled arg parsing and `--` discipline | the CLI idiom `bashy linux` follows. |
| `tools/windowswslocale` | a WSL-hosted locale provider for the conformance suite | assumes WSL already exists. |

There is no `bashy onboard`, and no recipe surface for Windows features: the
`script_recipes.go` recipes project the S9 trigger rows, not host convergence.

**Conclusion:** the transport, the image and the CLI idiom all exist. What is
missing is one verb that knows *how a host acquires a Linux environment* and
refuses to take privilege silently while doing it.

---

## 2. Live findings on elephant (2026-10-28, read-only)

Claimed with `bashy claim host:elephant.local:linux-runner`. Every command below
only **reports** state; nothing was installed, elevated-for-change, or
restarted.

```
$ ssh -p 2222 liqiang@elephant.local
bashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy-v1.0.0-rc.21-dev
uname        Windows_NT elephant 10.0.26300 Build 26300 x86_64
cmd /c ver   Microsoft Windows [Version 10.0.26300.9711]
EditionID    Professional
wsl.exe      PRESENT at C:\Windows\System32\wsl.exe
wsl --status exit 50
wsl -l -v    "The Windows Subsystem for Linux is not installed." (UTF-16LE)
sc query LxssManager  1060 — service does not exist
sc query vmcompute    1060 — service does not exist
net session  exit 0      -> the ssh session is ALREADY ELEVATED (no UAC hop needed)
systeminfo   Hyper-V Requirements: "A hypervisor has been detected."
dism /online /Get-FeatureInfo  (succeeded => elevated)
  VirtualMachinePlatform             State : Disabled
  Microsoft-Windows-Subsystem-Linux  State : Disabled
  HypervisorPlatform                 State : Disabled
  Microsoft-Hyper-V                  State : Disabled
Total Physical Memory  65,175 MB
```

What this settles:

1. **The `wsl: command not found` report was a red herring.** `wsl.exe` is
   present; the *component* is absent, and `wsl.exe` says so on stderr in
   **UTF-16LE** with exit 50 / 1. Any parser that does `string(out)` reads
   `W i n d o w s`. Hence `decodeConsole` and its test.
2. **No firmware blocker.** A hypervisor is already running (VBS/HVCI), which
   proves hardware virtualization is enabled in firmware. elephant is headless,
   so a BIOS trip would have been a hard stop — it is not needed.
3. **Windows 11 Professional** supports `VirtualMachinePlatform` and Hyper-V.
4. **The ssh session is already an administrator**, so the privileged steps need
   no interactive elevation — which makes the *guardrail*, not UAC, the only
   thing standing between the plan and the host.
5. **A restart is unavoidable.** Both optional features are `Disabled` and
   neither `LxssManager` nor `vmcompute` exists; enabling them requires a
   reboot. This is exactly the step the manager guardrail reserves.

### 2b. A separate Windows bug found on the way (not this story)

On elephant, a PATH-resolved external command inside a **pipeline** fails:

```
$ sc query vmcompute          # works
$ sc query vmcompute | cat    # bashy: line 1: sc: command not found
$ whoami | cat                # works (whoami is a coreutils applet)
$ reg query ... | cat         # command not found
```

Absolute paths work; in-process applets work; PATH lookup inside a pipeline does
not, and the error is attributed to the wrong pipeline element. Interleaving of
`echo` markers was also out of order. Filed separately — it does not affect this
verb (the gate's pipelines run *inside* Linux), but it would break any Windows
probe written as `cmd | grep`, which is why `probeLinuxHost` pipes nothing and
parses in Go.

---

## 3. The product path

One verb, provider-agnostic, in `internal/agentos/linuxrunner.go`:

```
bashy linux probe  [--json]              what Linux providers this host has (read-only)
bashy linux plan   [--json] [--provider P]  the ordered, idempotent converge steps
bashy linux up     [--allow-privileged] [--allow-reboot]
bashy linux status [--json]
bashy linux [run] -- CMD [ARGS...]
bashy linux --host [user@]HOST[:PORT] <any of the above>
```

### Providers, in preference order

| provider | where | how `run` works |
|---|---|---|
| `native` | `GOOS=linux` | the command runs here |
| `wsl` | Windows | `wsl.exe -d Ubuntu-24.04 -u root --cd <translated cwd> -- CMD` |
| `podman` | any OS | `bashy podman run --rm -i -v <cwd>:/w -w /w <image> CMD` |

`--provider` pins one and **errors** if the host cannot offer it — a gate result
is only comparable when the venue is known, so there is no silent fallback.

**The workspace is not copied.** On WSL a Windows path is already visible inside
the distribution under `/mnt/<drive>`, so `run` only translates the path
(`wslPath`); under podman the cwd is bind-mounted. "Workspace synced" is
therefore structural, not a copy step that can drift.

**One pinned distribution**, `Ubuntu-24.04`: a result only confirms a peer's
result when both ran the same userland, and the peer is noviwin1's WSL Ubuntu.

**No third-party image by default.** The podman provider's default image is
bashy's own `localhost/bashy:<ver>-linux-<arch>` from `bashy self image` (the
checksum-verified release artifact, FROM scratch). A third-party `--image` must
be a digest ref, same as `@contain(image:)`. Permissive-only / GPL-free rules
(`docs/licensing-supply-chain-policy.md`) are satisfied: the Ubuntu rootfs
`wsl --install` fetches is a runtime download + exec of separate programs, never
bundled or linked.

### Privilege is never taken silently

`plan` prints every step with its flags; `up` **refuses** and changes nothing
unless the authority is explicit:

* `Privileged` — needs administrator rights → `--allow-privileged`
* `Reboot` — authorizing it authorizes restarting the host → `--allow-reboot`
* `Restarts` — the step restarts the host *itself*, so `up` stops there and says
  to re-run afterwards

A refusal prints the exact argv, so a host's owner can authorize it or run it
themselves. Exit 2 (refused, nothing attempted), 1 (blocked or a step failed), 0.

### The plan on elephant today

`bashy linux plan` against the live facts above (this is the
`TestPlanElephantIsTheLiveFixture` fixture, byte-for-byte):

| # | step | argv | admin | reboot |
|---|---|---|---|---|
| 1 | `feature:VirtualMachinePlatform` | `dism.exe /online /Enable-Feature /FeatureName:VirtualMachinePlatform /All /NoRestart` | yes | yes |
| 2 | `feature:Microsoft-Windows-Subsystem-Linux` | `dism.exe /online /Enable-Feature /FeatureName:Microsoft-Windows-Subsystem-Linux /All /NoRestart` | yes | yes |
| 3 | `restart` | `shutdown.exe /r /t 60 /c "bashy linux: enabling WSL2"` | yes | **restarts** |
| 4 | `wsl-component` | `msiexec.exe /i wsl.<ver>.x64.msi /quiet /norestart` | yes | no |
| 5 | `wsl-default-version` | `wsl.exe --set-default-version 2` | no | no |
| 6 | `distro` | `wsl.exe --install --no-launch -d Ubuntu-24.04` | no | no |
| 7 | `toolchain` | install the release's linux bashy into the distro, `bashy go version` (managed Go 1.27.2), then link that go onto `PATH` | no | no |

Step 7 reuses `scratchArtifact` — the same checksum-verified path
`bashy self image` uses — so a Windows host provisions a Linux userland with no
Linux host to copy from. It ends by linking `$(bashy go env GOROOT)/bin/go` to
`/usr/local/bin/go`, for two reasons: a gate is written `go test ./...` — the
command a peer ran, and the command this verb exists to reproduce — and
`command -v go` is this step's own `Done` predicate, so without the link the
step never converges and `up` is not idempotent (§6.4).

### Idempotence

Every step carries a `Done` predicate read from the probe, so a converged host
has no pending steps and `up` prints `nothing to do` (exit 0). An
already-installed WSL is treated as proof that the optional features are on even
when `dism` could not be read (an unelevated probe), so a converged host run
without administrator rights still has **zero** pending privileged steps —
`TestPlanTrustsInstalledWSLOverUnknownFeatures`.

---


## 4. The cross-host hop needs three more things than a local run

`bashy linux` run **on** a host needs no workspace sync: WSL already sees the
Windows filesystem under `/mnt` and podman bind-mounts the cwd. A `--host`
**hop** is the one case where that is not true — the workspace is on the peer
that issued the command. Running the acceptance exposed three gaps, all fixed
here:

| flag | why the hop cannot work without it |
|---|---|
| `--remote-bashy CMD` | the hop re-issued `bashy linux …` on the host, but elephant's PATH bashy is the outpost-managed `v1.0.0-rc.21-dev`, which predates this verb: the hop died with `bashy: linux: command not found`. It is a **command**, split into words the way `BASHY_OCI` already is, so an `env VAR=V /path/bashy` bootstrap needs no second flag. |
| `--dir PATH` | the remote side ran in its ssh login directory. Dragon's cwd names nothing on elephant. |
| `--sync` | there was no way to get the workspace there at all. |

`--sync` sends, from a git worktree, only what git itself would show — tracked,
plus untracked that is not ignored, **plus each submodule's own such files**.
Three properties fall out of that choice:

* a gate result stays a property of the committed tree, not of somebody's
  build outputs;
* cloudbox goes as **17 MB of sources** instead of the 810 MB that `hub/bin`
  alone would add;
* `go.work`'s `use ./external/k3s` resolves, because a submodule's contents are
  listed in their own right (a gitlink has none in the parent's index).
  Measured: `GOWORK=off go list ./...` in `hub` fails with *updates to go.mod
  needed*, so the workspace — and therefore the submodule — is load-bearing.

The workspace **root** is the whole worktree, not the cwd, for the same reason:
`hub/` resolves through a `go.work` two levels above it, so syncing `hub/` alone
cannot build. The cwd's path relative to that root is where the remote runs.

The tree is **replaced**, not merged, so a file deleted locally cannot decide a
later gate result. The destination is always under `$HOME/.bashy/linux/workspaces/`
and the recursive removal refuses anything else.

## 5. Live bring-up of elephant, 2026-10-28 (operator-approved)

Approval: `s522-manager` note, sprint #522 thread, 10-28 18:07 — steps 1–7
including the reboot. Host claimed as `host:elephant.local:linux-runner`
throughout. noviwin1 untouched.

elephant's canonical bashy (`C:\Users\liqia\AppData\Local\outpost\bashy.EXE`,
the outpost agent's own binary) was **not** replaced. The build under test was
pushed to `C:/Users/liqia/.bashy/linux/bin/` and named with `--remote-bashy`.

```sh
# the build under test, and the linux artifact its toolchain step installs
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags -w -o bashy-s522-10-win-amd64.exe ./cmd/bashy
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags -w -o bashy-s522-10-linux-amd64   ./cmd/bashy
scp -P 2222 bashy-s522-10-win-amd64.exe bashy-s522-10-linux-amd64 \
    'liqiang@elephant.local:C:/Users/liqia/.bashy/linux/bin/'

RB='env BASHY_SCRATCH_BIN=C:/Users/liqia/.bashy/linux/bin/bashy-s522-10-linux-amd64 /c/Users/liqia/.bashy/linux/bin/bashy-s522-10-win-amd64.exe'
H='--host liqiang@elephant.local:2222 --remote-bashy'
```

`scp` needs a **Windows** remote path (`C:/…`). `/c/Users/…` is bashy's own
shell form and the Windows sftp server rejects it — `remote mkdir "/c/…": No
such file or directory`.

### 5.1 probe + plan + the default refusal (read-only)

```
$ bashy linux $H "$RB" probe
host       windows/amd64
providers  wsl
selected   wsl
windows    edition=Professional elevated=true hypervisor=true firmware-virt=yes
wsl        stub=true installed=false distros=none go-in-distro=false
feature    VirtualMachinePlatform=Disabled
feature    Microsoft-Windows-Subsystem-Linux=Disabled
podman     reachable=false image=unknown present=false

$ bashy linux $H "$RB" up          # no authority given
bashy linux: refusing to continue: step "feature:VirtualMachinePlatform" needs administrator rights.
  step: dism.exe /online /Enable-Feature /FeatureName:VirtualMachinePlatform /All /NoRestart
Nothing was changed. Re-run with --allow-privileged once the host's owner has agreed.
```

`plan` printed the approved seven steps byte-for-byte.

### 5.2 the authorized privileged hop, and the reboot

```
$ bashy linux $H "$RB" up --allow-privileged --allow-reboot     # 18:22:58Z
bashy linux: feature:VirtualMachinePlatform — …
bashy linux: feature:Microsoft-Windows-Subsystem-Linux — …
bashy linux: restart — the Windows optional features above only take effect after a restart
  shutdown.exe /r /t 60 /c bashy linux: enabling WSL2
bashy linux: this host is restarting; re-run `bashy linux up` afterwards to finish
```

elephant went down at **18:24:34Z** (the 60 s warning) and answered ssh again at
**18:24:40Z** — `uptime` reported `up 0 min`, so it really restarted; the
window is that short because the host boots from NVMe. Re-probe:

```
feature    VirtualMachinePlatform=Enabled
feature    Microsoft-Windows-Subsystem-Linux=Enabled
```

### 5.3 The inbox `wsl.exe` cannot install WSL — the component comes from Microsoft's MSI

This is the one place the approved plan was **wrong**, and it is a fleet-wide
fact, not an elephant quirk. With both optional features enabled and the host
restarted, `LxssManager` still did not exist, and **every** install route
through the inbox stub answered the same thing and exited 1:

```
$ wsl.exe --update                        -> exit 1
$ wsl.exe --install                       -> exit 1
$ wsl.exe --install --no-launch           -> exit 1
$ wsl.exe --install --no-distribution     -> exit 1
    The Windows Subsystem for Linux is not installed. You can install by
    running 'wsl.exe --install'.
```

The stub's whole argument list is `--install/--update/--status/--help` (its own
`--help` proves it), and its install path wants the Microsoft Store, which an
unattended session on a server-shaped host does not have. A fleet cannot
converge through a binary that tells it to run the command that just failed.

Microsoft publishes the component as a plain **MSI** for exactly this
offline/enterprise case. Verified live, then made the step:

```
$ msiexec.exe /i wsl.3.0.3.0.x64.msi /quiet /norestart     -> EXIT=0
$ wsl.exe --status
Default Version: 2
WSL version: 3.0.3.0        Kernel version: 6.18.54.1
```

The step resolves the asset through `binmgr.ResolveGitHub` against
`microsoft/WSL` — the same checksum-verified path `bashy self image` uses for
bashy's own artifact; every asset in that release carries a `sha256:` digest in
the GitHub API. WSL is MIT-licensed and this is a runtime download + exec of a
separate program, so nothing propagates: the same standing podman and ollama
already have under `docs/licensing-supply-chain-policy.md`. `/norestart` is
deliberate — the features already took their restart, and a second one here
would be outside what `up` was authorized to do.

### 5.4 distro + toolchain, and the idempotence it first failed

```
$ bashy linux $H "$RB" up --allow-privileged
bashy linux: wsl-default-version — …    wsl.exe --set-default-version 2
bashy linux: distro — …                 wsl.exe --install --no-launch -d Ubuntu-24.04
bashy linux: toolchain — …
bashy linux: wsl is set up
```

…and then `status` said **`not ready via wsl — 1 step(s) left: toolchain`**.
The toolchain step installed bashy and provisioned Go 1.27.2, but left no `go`
on `PATH`, and `command -v go` is that step's own `Done` predicate — so `up`
re-provisioned on every run and the no-op property the whole plan rests on was
absent. Fixed by linking the managed toolchain onto `PATH`
(`linkDistroGo`), which a gate written `go test ./...` needs anyway:

```
$ wsl -d Ubuntu-24.04 -u root -- bashy go env GOROOT
/root/.cache/bashy/bin/go/1.27.2/go
$ ln -sfn /root/.cache/bashy/bin/go/1.27.2/go/bin/go /usr/local/bin/go
```

### 5.5 Converged, and `up` is a no-op with no authority at all

```
$ bashy linux $H "$RB" status
linux: ready via wsl                                  # exit 0

$ bashy linux $H "$RB" up                             # no --allow-* flags
bashy linux: wsl is already set up here; nothing to do # exit 0

$ bashy linux $H "$RB" -- sh -lc 'uname -srm; go version; nproc; grep MemTotal /proc/meminfo'
Linux 6.18.54.1-microsoft-standard-WSL2 x86_64
go version go1.27.2 linux/amd64
32
MemTotal:       32679120 kB
```

That last command is also the proof of the quoting fix in §6.2: a `;`-separated
command survived the hop intact.

## 6. Defects found by running it, each with a test

1. **The hop required the verb on the host.** Fixed with `--remote-bashy`
   (§4) — `TestRunLinuxHostHopUsesRemoteBashyAndDir`,
   `TestLinuxRemoteBashyIsACommandNotJustAPath`.
2. **ssh has no argv.** It concatenates its trailing arguments and the remote
   **login shell** re-parses them, so an unquoted hop silently lost the
   command's own quoting: `run -- sh -lc 'uname -a; free -g'` ran `free -g` on
   the *Windows* host. `shellJoin` now renders the remote argv as one word-for-
   word command line, leaving safe words bare so a printed hop stays pasteable
   — `TestHostHopQuotesTheRemoteCommand`,
   `TestShellJoinLeavesSafeWordsBareAndEscapesQuotes`.
3. **`wsl --update` cannot bootstrap the stub** (§5.3) —
   `TestWSLComponentStepUsesTheMSINotTheStub`,
   `TestWSLMSIAssetMatchPicksThisArchitecture` (one release ships x64 and
   arm64 MSIs plus an msixbundle and a nupkg, and both MSIs end in `.msi`).
4. **The toolchain step never converged** (§5.4) —
   `TestToolchainPutsTheManagedGoOnPath`, which also asserts that an unreadable
   `GOROOT` fails the step rather than reporting success.
5. **`/tmp` is a symlink on macOS.** `git rev-parse --show-toplevel` answered
   `/private/tmp/cb-dev` while the cwd was `/tmp/cb-dev/hub`, so the relative
   path came out `../../../tmp/cb-dev/hub` and the remote chdir landed off the
   end of the synced tree. Both sides now go through `EvalSymlinks`, and a cwd
   genuinely outside its root is refused instead of producing a `..` path —
   `TestLocalLinuxWorkspaceResolvesSymlinkedRoots`,
   `TestLocalLinuxWorkspaceRefusesACwdOutsideItsRoot`.

### 6b. The Windows pipeline bug of §2b, reconfirmed

While probing, `sc query LxssManager | head -5` and
`powershell.exe -Command … | head -5` both reported `command not found`, while
the same commands without a pipeline worked. PATH lookup inside a pipeline is
still broken on Windows, and still attributed to the wrong pipeline element. It
does not affect this verb (the gate's pipelines run *inside* Linux, and
`probeLinuxHost` pipes nothing and parses in Go), and it remains filed
separately.

## 7. Acceptance: the cloudbox hub Linux gate, from Dragon, on elephant

One command. The tree is a clean `origin/dev` worktree of cloudbox (60b4711)
with its submodules initialised, so the operator's own checkout was never
touched:

```sh
git -C ~/projects/poc/dhnt/cloudbox worktree add --detach /tmp/cb-dev 60b4711
git -C /tmp/cb-dev submodule update --init --recursive

bashy awd /tmp/cb-dev/hub -- \
  bashy linux --host liqiang@elephant.local:2222 --remote-bashy "$RB" --sync -- \
    env GOTOOLCHAIN=local GOFLAGS=-p=4 go test -short ./...
```

```
=== ACCEPTANCE start 19:02:08Z
bashy linux: sync 2267 file(s) from /private/tmp/cb-dev to elephant.local:/c/Users/liqia/.bashy/linux/workspaces/cb-dev-4d80b175
ok  github.com/qiangli/cloudbox/api/entity                   (cached)
ok  github.com/qiangli/cloudbox/cmd                          (cached)
ok  github.com/qiangli/cloudbox/cmd/dks-metrics-aggregator    (cached)
ok  github.com/qiangli/cloudbox/internal/api                 36.877s
ok  github.com/qiangli/cloudbox/internal/appstore            (cached)
…
ok  github.com/qiangli/cloudbox/internal/server              48.690s
ok  github.com/qiangli/cloudbox/internal/version             (cached)
GATE_EXIT=0
=== end 19:03:30Z
```

**38 packages `ok`, zero `FAIL`, exit 0**, on
`Linux 6.18.54.1-microsoft-standard-WSL2 x86_64` with `go1.27.2 linux/amd64`.

This **matches the noviwin1 WSL baseline** the manager holds (v0.18.43 gate
passed there): the same package set green, and `internal/api` at 36.9 s against
noviwin1's ~35 s. The two hosts are comparable because the distribution is
pinned — both ran Ubuntu-24.04 under WSL2. **noviwin1 was not touched.**

The first run of this command (18:49:24Z–18:59:25Z) compiled the whole
k3s/k8s dependency tree from cold and populated the distro's module cache;
the run above reused it, which is why most packages report `(cached)`.

### Re-running setup is a no-op (§5.5)

```
$ bashy linux $H "$RB" up          # no --allow-privileged, no --allow-reboot
bashy linux: wsl is already set up here; nothing to do     # exit 0
```

## 8. Status

**Done**

* The verb, the three providers and the privilege refusals (Story e0d41dec,
  previous run).
* `--remote-bashy`, `--dir`, `--sync` — everything a *cross-host* hop needs
  (§4), with tests.
* The WSL component installed from Microsoft's MSI instead of through the inbox
  stub, which cannot install itself (§5.3).
* The toolchain step links the managed Go onto `PATH`, which is what makes `up`
  idempotent and a `go test` gate runnable as written (§5.4).
* The five defects of §6, each with a red/green test.
* elephant converged and the acceptance gate green from Dragon (§7).

**Not done / next, in priority order**

1. **elephant runs an unreleased build pushed to
   `C:/Users/liqia/.bashy/linux/bin/`, named with `--remote-bashy`.** The
   fleet-convergence promise is only kept once this ships in a bashy release
   and elephant's outpost bashy carries the verb; then `--remote-bashy` is
   needed for bootstrap only. `BASHY_SCRATCH_BIN` is likewise a stand-in for
   the release artifact this build has no version stamp to name.
2. Replace the ssh hop with the in-process peer channel
   (`DialInstalledPeerChannel`, `remote_exec.go`) once `bashy remote install`
   has run on elephant, so the hop needs no host ssh client.
3. The podman provider on Windows is now actually reachable on elephant
   (VirtualMachinePlatform is on) and untested here; the wsl provider is what
   the acceptance measured.
4. A bashy-managed Linux VM provider for hosts where WSL cannot exist.
5. Fix the Windows pipeline PATH-lookup bug (§2b, §6b).
