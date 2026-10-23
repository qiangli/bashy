# Setting up bashy on a new machine

This guide gets a new Windows, macOS or Linux machine from "nothing installed" to
"bashy + outpost installed, paired, and reachable". Two questions pick your path:

|                       | **You do it** (no AI tools)  | **An AI agent does it**   |
|-----------------------|------------------------------|---------------------------|
| **On the new machine**     | [A. Manual, local](#a-manual-on-the-new-machine) | [B. Agent, local](#b-ai-agent-on-the-new-machine) |
| **From another machine**   | [C. Manual, remote](#c-manual-from-another-machine) | [D. Agent, remote](#d-ai-agent-from-another-machine) |

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

## A. Manual, on the new machine

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

## B. AI agent, on the new machine

Use this when an agent (Claude Code, Codex, ycode, …) runs on the new machine.

1. Download and extract the archive (only the human can do the very first
   download).
2. Give the agent this guide and say where to pair it, for example:
   > Install bashy from this folder following docs/setup.md path A, and pair it
   > with satellite https://&lt;your-satellite&gt; as `<new-host>`.
3. The agent runs path A. When it needs the pairing code it asks you with
   `bashy ask`; type the code into that prompt (never paste it into the chat).

## C. Manual, from another machine

Use this when you want to drive the setup from your laptop.

1. **On the new machine**, extract the archive and start the bootstrap SSH
   server (keep the window open until setup is done):
   ```sh
   outpost sshd            # Windows: .\outpost.exe sshd
   ```
   It listens on port **2222**, accepts the machine's login name and OS password,
   and announces itself over mDNS as `<host>.local`. On Windows, if the network is
   set to **Public**, the firewall blocks it: set it to Private first (Settings,
   Network, or as Administrator `Set-NetConnectionProfile -NetworkCategory Private`).

2. **From your laptop**, connect with any SSH client and run path A there:
   ```sh
   ssh -p 2222 <login-name>@<host>.local
   ```
   The session runs bashy, so the same commands work on every OS. Copy files with
   `scp -P 2222` / `sftp -P 2222` if needed.

3. Close the bootstrap `outpost sshd` window when pairing is done.

## D. AI agent, from another machine

Use this when an agent on your laptop sets the machine up.

1. **On the new machine**: extract the archive and run `outpost sshd`, as in
   C.1.
2. **Tell the agent** the host name (`<host>.local`), the login name and where to
   pair it, for example:
   > Set up `<host>.local` (login `<login-name>`, bootstrap sshd on 2222) following
   > docs/setup.md path D and pair it with satellite https://&lt;your-satellite&gt;.
3. The agent then:
   - asks you for the OS password with `bashy ask` (type it there, never in chat)
     and connects to port 2222;
   - installs bashy and outpost into the standard location (path A step 1);
   - mints a pairing code from your laptop (`bashy satellite invite …` or the
     portal) and passes it to the new machine as a short-lived file that the
     install step reads and deletes;
   - runs `bashy login --server … --code …` there and verifies the host is online.
4. Close the bootstrap `outpost sshd` window.

Agent notes: run Windows PowerShell with `-NonInteractive -InputFormat None` and
redirect stdin from `/dev/null`; PowerShell 5 over SSH never sees end-of-input on
piped stdin, so never pass secrets that way.

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
