# Setting up bashy on a new machine

This guide takes you from nothing to a working fleet: an account, your own
satellite, and your machines (PCs and cloud VMs) paired with it. It covers
Windows, macOS and Linux.

## The onboarding journey

| Stage | What happens | Recommended: an AI agent does it | Manual |
|---|---|---|---|
| 1. Account | sign up / log in at ai.dhnt.io and create an owner token for your agents | you sign up in the browser, then hand the token to the agent with `bashy ask` | [stage 1](#stage-1-account) |
| 2. Satellite | create your own satellite on your cloud account (DigitalOcean today) | the agent provisions it through the portal's provisioning API | [stage 2](#stage-2-satellite) |
| 3. This machine | install bashy here and pair it with the satellite | the agent runs [path 2](#2-ai-agent-on-the-new-machine) | [path 1](#1-manual-on-the-new-machine) |
| 4. More machines | add PCs and cloud VMs to the satellite | [path 3](#3-bootstrap-then-an-ai-agent-from-another-machine) for PCs, [cloud servers](#cloud-servers-digitalocean-aws-azure-google-cloud-) for VMs | [path 1](#1-manual-on-the-new-machine) on each PC; cloud-init for VMs |

With an AI agent you only ever do three things yourself: sign up in the browser,
answer `bashy ask` prompts (tokens, pairing codes, passwords), and, for a PC
the agent cannot reach yet, start the bootstrap command on it.

### Stage 1: account

1. Sign up or log in at <https://ai.dhnt.io> (GitHub, Google, LinkedIn, …).
2. Create an owner token for your agents: Cloudbox, then Settings, then Access
   Tokens, preset **Fleet owner (agents)**. Copy it once.
3. Give it to your agent through `bashy ask` (never paste it into a chat). The
   agent keeps it as `BASHY_FLEET_TOKEN` and can mint narrower tokens from it
   with `bashy token create`.

### Stage 2: satellite

A satellite is your own control plane: it runs on your cloud account, at your
cost, and your machines pair with it instead of the shared portal.

1. Give the portal a scoped provider token for your cloud account (Cloudbox,
   then Satellites; for DigitalOcean a custom-scoped API token without delete
   rights).
2. Create the satellite: Cloudbox, then Satellites, then Provision, or let the
   agent call the provisioning API (`POST /api/v1/provisioning/requests`, owner
   token). Follow it with `bashy infra request ls` / `status`; it is ready when
   the request says `installed`.
3. Note its URL (for example `https://<ip-dashed>.sslip.io`): stages 3 and 4
   pair with it.

### Stage 3 and 4: machines

The rest of this guide. Three paths add a PC; cloud VMs are set up at first boot:

| Path | Where the work happens | Who does it |
|---|---|---|
| [1. Manual](#1-manual-on-the-new-machine) | on the new machine | you |
| [2. AI agent on the machine](#2-ai-agent-on-the-new-machine) | on the new machine | an agent running there, with you answering its prompts |
| [3. Bootstrap, then an AI agent remotely](#3-bootstrap-then-an-ai-agent-from-another-machine) | you start one command on the new machine; the rest from another machine | an agent on your laptop |
| [Cloud servers](#cloud-servers-digitalocean-aws-azure-google-cloud-) | the VM's first boot | an agent (or you) generates the user-data |

Every path uses only bashy and outpost. No OpenSSH server, package manager or
other tool is needed on the new machine.

Status: this describes what works **today**. A shorter flow is planned
([coming next](#coming-next)).

The steps in this guide are also a Bash# recipe you can read as a skill and run as
a script: [`skills/bashy/recipes/outpost-setup.dag.md`](../skills/bashy/recipes/outpost-setup.dag.md)
(`bashy dag -f outpost-setup.dag.md setup …`).

## Before you start

**1. Download** the archive for the new machine from the
[Releases](https://github.com/qiangli/bashy/releases) page:

| Machine | Asset |
| --- | --- |
| Windows x86-64 | `bashy-windows-amd64.zip` |
| Windows arm64 | `bashy-windows-arm64.zip` |
| macOS Apple Silicon | `bashy-darwin-arm64.tar.gz` |
| macOS Intel | `bashy-darwin-amd64.tar.gz` |
| Linux x86-64 | `bashy-linux-amd64.tar.gz` |
| Linux arm64 | `bashy-linux-arm64.tar.gz` |

Each archive holds `bashy`, `outpost`, `bash` and `sh` (`.exe` on Windows).

**2. Know where it will be paired.** A machine joins either the Tessaro portal
or one of your satellites. You will need a one-time **pairing code** (valid a few
minutes) from one of these:

- the portal: Cloudbox, then Hosts, then "Generate invite code";
- an already-paired machine, for a satellite:
  `bashy satellite invite --satellite https://<your-satellite> --name <new-host>`.

**3. Know the login name** of the account on the new machine (`whoami`). On
Windows this is the account name (for example `alexandra`), which can differ from
the profile folder name under `C:\Users` (for example `alexa`).

## 1. Manual, on the new machine

Run these in any terminal on the new machine: its own keyboard and screen, or a
remote session you already have on it (for example a cloud VM's console or SSH).
Driving the setup from another machine is what path 3's agent is for.

1. **Extract and install** into the standard per-user location.

   Windows (PowerShell, in the extracted folder):
   ```powershell
   .\bashy.exe self install --dir "$env:LOCALAPPDATA\outpost"
   [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path","User") + ";$env:LOCALAPPDATA\outpost", "User")
   ```
   Open a new terminal afterwards so the updated `Path` applies.

   macOS / Linux:
   ```sh
   tar -xzf bashy-<os>-<arch>.tar.gz
   ./bashy self install --dir ~/.local/bin      # make sure ~/.local/bin is on PATH
   ```
   On macOS, a browser download is quarantined; clear it once with
   `xattr -dr com.apple.quarantine ~/.local/bin/bashy ~/.local/bin/outpost`.

2. **Pair** with the code from "Before you start":
   ```sh
   bashy login --code <code>                                   # Tessaro portal
   bashy login --server https://<your-satellite> --code <code> # a satellite
   ```
   Pairing registers outpost to start automatically: a Task Scheduler task on
   Windows, a launchd agent on macOS, a systemd user unit on Linux. On a headless
   Linux machine also run `loginctl enable-linger $USER` so it runs without a login.

3. **Check**: `outpost status` shows `state active (paired as <name>)` and the
   portal or satellite lists the host as online.

## 2. AI agent, on the new machine

Use this when an agent (Claude Code, Codex, ycode, …) runs on the new machine.

1. Download and extract the archive (only the human can do the very first
   download).
2. Give the agent this guide and say where to pair it, for example:
   > Install bashy from this folder following docs/setup.md path 1, and pair it
   > with satellite https://&lt;your-satellite&gt; as `<new-host>`.
3. The agent runs path 1. When it needs the pairing code it asks you with
   `bashy ask`; type the code into that prompt (never paste it into the chat).

## 3. Bootstrap, then an AI agent from another machine

Use this when an agent on your laptop sets the machine up. The only thing done
on the new machine is starting a temporary SSH server.

1. **On the new machine**, extract the archive and start the bootstrap SSH
   server (keep the window open until setup is done):
   ```sh
   outpost sshd            # Windows: .\outpost.exe sshd
   ```
   It listens on port **2222**, accepts the machine's login name and OS password,
   and announces itself over mDNS as `<host>.local`. On Windows, if the network is
   set to **Public**, the firewall blocks it: set it to Private first (Settings,
   Network, or as Administrator `Set-NetConnectionProfile -NetworkCategory Private`).
2. **Tell the agent** the host name (`<host>.local`), the login name and where to
   pair it, for example:
   > Set up `<host>.local` (login `<login-name>`, bootstrap sshd on 2222) following
   > docs/setup.md path 3 and pair it with satellite https://&lt;your-satellite&gt;.
3. The agent then:
   - asks you for the OS password with `bashy ask` (type it there, never in chat)
     and connects with `ssh -p 2222 <login-name>@<host>.local`; the session runs
     bashy, so the same commands work on every OS;
   - installs bashy and outpost into the standard location (path 1, step 1);
   - mints a pairing code on your laptop (`bashy satellite invite …` or the
     portal) and passes it to the new machine as a short-lived file that the
     install step reads and deletes;
   - runs `bashy login --server … --code …` there and verifies the host is online.
4. Close the bootstrap `outpost sshd` window.

Agent notes: run Windows PowerShell with `-NonInteractive -InputFormat None` and
redirect stdin from `/dev/null`; PowerShell 5 over SSH never sees end-of-input on
piped stdin, so never pass secrets that way. Copy files with `scp -P 2222` /
`sftp -P 2222`.

## Cloud servers (DigitalOcean, AWS, Azure, Google Cloud, …)

A cloud VM joins a satellite as one more outpost, next to your Windows, macOS
and Linux machines. There is nobody at its console, so it is set up at first
boot from its user-data:

```sh
bashy fetch https://raw.githubusercontent.com/qiangli/bashy/main/skills/bashy/recipes/outpost-setup.dag.md -o outpost-setup.dag.md
bashy dag -f outpost-setup.dag.md cloud-init SATELLITE=https://<your-satellite> NAME=<vm-name> RELEASE_TAG=<tag>
```

This writes `<vm-name>.user-data.sh` (mode 0600). Pass it as the VM's user-data
(DigitalOcean user data, AWS EC2 user data, Azure custom data, Google Cloud
startup-script) when creating a Linux VM. On first boot it downloads bashy for the
VM's architecture, checks the release checksum, installs bashy and outpost into
`/usr/local/bin`, pairs with the satellite using a single-use invite (valid 30
minutes by default, `INVITE_TTL`), and registers outpost as a boot-time service.
Delete the user-data file after the VM is created: it holds the invite.

Then confirm it came online (polls the satellite, up to 15 minutes):

```sh
bashy dag -f outpost-setup.dag.md verify-remote SATELLITE=https://<your-satellite> NAME=<vm-name>
```

`cloud-init`, `invite` and `verify-remote` act for the satellite's owner: they
read the owner's Bearer token from `BASHY_FLEET_TOKEN` (`invite` and
`cloud-init` fall back to an already-paired machine's own token). If a VM registers but stays
offline, read its boot log (`/var/log/cloud-init-output.log`) from the provider
console.

Cleaning up a test VM with `bashy infra trash` needs the provider token in the
provider's usual variable (for DigitalOcean `DIGITALOCEAN_ACCESS_TOKEN`) and the
cloudbox ownership tag `bashy-owner-<owner>` on the VM; add it when you create the
VM (or later with `bashy infra harden --droplet <id> --tag bashy-owner-<owner>`).

If you already have the provider's SSH access to a VM, path 1 works there too.

## Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| Host answers, then vanishes (Windows) | network profile **Public**: firewall drops inbound | set it to Private |
| `Permission denied` with the right password | profile folder name used instead of the login name | use `whoami` on the machine |
| `outpost is already running (pid N)` but nothing runs | stale pidfile after a reboot (fixed in outpost 0f2c3eb and later) | upgrade outpost |
| `outpost scan` finds nothing | discovery is off on the other outposts | connect by `<host>.local` |
| outpost stops after you log out (Linux) | systemd user units stop without linger | `loginctl enable-linger $USER` |

## Coming next

- **`bashy setup` + `bashy fleet adopt`**: the human runs one command and reads
  a setup code off the screen; an agent on another machine finishes everything,
  with no OS password, user name or bootstrap window.
- **Supervised `outpost.sshd`** on port **2223** with key authentication, its own
  firewall rule and mDNS by default (the bootstrap keeps 2222).
- **Windows**: `winget install dhnt.bashy` and Authenticode-signed binaries.
- `bashy self install` will default to the standard per-user directory, so `--dir`
  becomes optional.
