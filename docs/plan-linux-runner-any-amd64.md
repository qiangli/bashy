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
| 4 | `wsl-update` | `wsl.exe --update` | yes | no |
| 5 | `wsl-default-version` | `wsl.exe --set-default-version 2` | no | no |
| 6 | `distro` | `wsl.exe --install --no-launch -d Ubuntu-24.04` | no | no |
| 7 | `toolchain` | install the release's linux bashy into the distro, then `bashy go version` (managed Go 1.27.2) | no | no |

Step 7 reuses `scratchArtifact` — the same checksum-verified path
`bashy self image` uses — so a Windows host provisions a Linux userland with no
Linux host to copy from.

### Idempotence

Every step carries a `Done` predicate read from the probe, so a converged host
has no pending steps and `up` prints `nothing to do` (exit 0). An
already-installed WSL is treated as proof that the optional features are on even
when `dism` could not be read (an unelevated probe), so a converged host run
without administrator rights still has **zero** pending privileged steps —
`TestPlanTrustsInstalledWSLOverUnknownFeatures`.

---

## 4. Dry run against elephant (what to run, in order)

Steps 1–2 are read-only and were **already executed live** (§2). Step 3 is the
guardrail stop.

```sh
# 1. claim the host for the duration
bashy claim host:elephant.local:linux-runner --intent "S522.10 linux-runner"

# 2. READ-ONLY — safe on a machine the operator is using
bashy linux --host liqiang@elephant.local:2222 probe --json
bashy linux --host liqiang@elephant.local:2222 plan

# 3. GUARDRAIL STOP — privileged + reboot. Refused by default:
bashy linux --host liqiang@elephant.local:2222 up
#   -> exit 2, "refusing to continue: step "feature:VirtualMachinePlatform"
#      needs administrator rights ... Nothing was changed."
# Post the table in §3 to `bashy sprint comment 522` and WAIT for the operator.

# 4. ONLY after the operator agrees, in two authorized hops:
bashy linux --host liqiang@elephant.local:2222 up --allow-privileged --allow-reboot
#   -> enables both features, restarts elephant, stops and says to re-run
bashy linux --host liqiang@elephant.local:2222 up --allow-privileged
#   -> wsl --update, --set-default-version 2, install Ubuntu-24.04, toolchain

# 5. acceptance
bashy linux --host liqiang@elephant.local:2222 status
bashy awd ~/src/cloudbox/hub -- \
  bashy linux --host liqiang@elephant.local:2222 -- \
    env GOTOOLCHAIN=local go test -short ./...
# compare against noviwin1's WSL result. DO NOT TOUCH noviwin1.

# 6. re-running step 4 must be a no-op:
bashy linux --host liqiang@elephant.local:2222 up   # -> "nothing to do", exit 0
```

---

## 5. Status

**Done in this story**

* `internal/agentos/linuxrunner.go` — the verb: probe, plan, up, status, run;
  native/wsl/podman providers; the `--host` hop; UTF-16LE console decoding;
  privilege/reboot refusals.
* `internal/agentos/linuxrunner_test.go` — red/green tests over an injected
  host, including elephant's live facts as the plan fixture, the probe's
  read-only property, both refusals, the restart stop, idempotence, and the
  acceptance argv.
* Registered: `verbSynopsis`, `bashyOwnedVerbAtlas`, `directFrontDoorVerbs`
  (direct-only — a bare `linux` shim would be wrong), the dispatch arm, and the
  `TestE2EAllListedCommandsDispatch` native set.
* Live read-only probe of elephant (§2) and the pipeline bug found doing it
  (§2b).

**Blocked on the manager/operator guardrail**

* Steps 4–6 of §4 on elephant. `up` refuses on purpose until then; the exact
  steps are posted to sprint #522.

**Next, in priority order**

1. Replace the `--host` ssh hop with the in-process peer channel
   (`DialInstalledPeerChannel`, `remote_exec.go`) once `bashy remote install`
   has run on elephant, so the hop needs no host ssh client.
2. Streamed output for the `--host` hop: `sshclient.ExecOptions` captures with a
   1 MiB cap, which a long `go test` can exceed.
3. A bashy-managed Linux VM provider for hosts where WSL cannot exist (Windows
   Home without the feature set, or a firmware-blocked host) — the same plan
   shape with a different provider.
4. Fix the Windows pipeline PATH-lookup bug in §2b.
