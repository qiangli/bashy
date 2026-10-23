# Setting up bashy on a new machine

This guide gets a new Windows, macOS or Linux machine from "nothing installed" to
"bashy + outpost installed, paired, and reachable". There are three ways to do it:

| Path | Where the work happens | Who does it |
|---|---|---|
| [1. Manual](#1-manual-on-the-new-machine) | on the new machine | you |
| [2. AI agent on the machine](#2-ai-agent-on-the-new-machine) | on the new machine | an agent running there, with you answering its prompts |
| [3. Bootstrap, then an AI agent remotely](#3-bootstrap-then-an-ai-agent-from-another-machine) | you start one command on the new machine; the rest from another machine | an agent on your laptop |

Every path uses only bashy and outpost. No OpenSSH server, package manager or
other tool is needed on the new machine.

Status: this describes what works **today**. A shorter flow is planned
([coming next](#coming-next)).

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
