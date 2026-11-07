---
id: 01a1b809-6399-729b-8f2a-4e1f077c6b3a
seq: 12
form: page
type: runbook
title: 'Set up outpost: install, pair, publish apps, toggle built-ins, elevate, keep a host private'
description: 'End-to-end outpost setup for a host joining a satellite: installer, outpost register, status, apps add/list/rm, builtins set and service units, outpost connect/ssh/scp, and what is shared by default — WHEN installing or pairing a new host, publishing a local web app, or locking a host down (keywords: outpost, install, register, pair, pairing code, apps add, require-login, trust-cloud-identity, builtins, service, connect, elevation, ssh, scp, private, share)'
tags:
    - outpost
    - install
    - pairing
    - apps
    - builtins
    - elevation
    - privacy
status: candidate
evidence: '2026-11-07: every command and flag checked against outpost <cmd> --help and bashy service list on the current release; defaults from outpost/docs/settings.md and docs/pairing.md; install one-liners from the bashy README'
source:
    tool: claude
created: "2026-11-07T22:03:05Z"
---

## Install

```sh
curl -fsSL https://dhnt.io/install.sh | sh          # macOS / Linux
```

```powershell
irm https://dhnt.io/install.ps1 | iex               # Windows PowerShell
```

## Pair with a satellite

Get a one-time code (short-lived, single use) from the satellite owner console: Hosts, Generate invite code. Pair with your OWN account (see the satellite sharing page):

```sh
outpost register --code "$PAIRING_CODE" --server "$SATELLITE_URL" --name "$(hostname)"
```

`--server` defaults to the official portal and `--name` to this machine's hostname. Pairing into the default config dir installs the per-user service and starts the daemon. Save the printed recovery code (also stored under `${XDG_CONFIG_HOME:-$HOME/.config}/matrix/`); it is the only way to re-pair if both the access token and the tunnel are lost. For start-at-boot without login, run `outpost service install` (admin).

## Check

```sh
outpost status            # pairing, built-ins, outbound (--json available)
outpost peers status      # other hosts on the account: online/offline
outpost doctor            # boot readiness
```

## Publish apps

```sh
outpost apps add "$APP" --url http://127.0.0.1:$PORT --require-login [--trust-cloud-identity]
outpost apps list                 # this host;  outpost apps list "$HOST_NAME" for a paired host
outpost apps rm "$APP"
```

- `--require-login` limits the app to signed-in, authorized users (owner or sharee). Without it the app is not gated by sign-in; leave it off only for something meant to be public.
- `--trust-cloud-identity` forwards the vouched user as `Remote-User`/`Remote-Email` (an app with its own SSO reads it; `outpost apps secret "$APP"` prints the HMAC secret it needs).
- `--elevation-required` additionally demands the host's OS password.
- App URL on a satellite: `$SATELLITE_URL/matrix/h/$HOST_NAME/app/$APP/`; it needs sign-in and elevation.

## Built-in routes and services

```sh
outpost builtins show
outpost builtins set --shell off --desktop off --clipboard off      # on|off each; only flags passed change
outpost builtins set --service files=on --service ollama=off        # service units, live
bashy service list                                                  # names the units
```

## Elevate, ssh, scp

```sh
outpost connect "$HOST_NAME"                     # OS password once; cookie lasts 1 h idle / 8 h absolute
outpost ssh "$HOST_NAME" uname -a
outpost scp ./file "$HOST_NAME":/tmp/file        # --safe for binaries (staged + atomic rename)
```

A host shared with you needs no password: the share grant is the authority. Agents pass the password on stdin with `--stdin`.

## What is shared by default, and keeping a host private

- Built-in routes (shell, desktop, clipboard, ssh, sftp) are ON by default, but only reachable through the satellite by the host owner after elevation and by accounts the owner shares with. Service units ship disabled, except the LAN sshd on port 2222 (mDNS-advertised). Cluster join is opt-in.
- Nothing is shared with other members until the owner shares a host or app.
- To keep a host private: do not share it; turn off routes you do not use (`builtins set --shell off --desktop off --clipboard off --ssh off --sftp off`); `--service sshd=off` closes the LAN listener; publish apps only with `--require-login` (and `--elevation-required` where it matters). A machine that should only ssh out can pair with `outpost register --client-only` (it can never be a share target).

## More

- Several satellites at once: `outpost-profile up` (see the outpost-profile page).
- Never `pkill`/`killall` outpost by pattern: it is the only remote path to the host. Use `outpost restart` / `outpost disable`.
