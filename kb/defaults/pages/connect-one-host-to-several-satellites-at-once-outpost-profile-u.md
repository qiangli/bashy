---
id: 01a1b7c4-155c-75b8-8b16-8bb62ccfcffd
seq: 8
form: page
type: runbook
title: Connect one host to several satellites at once (outpost-profile up)
description: 'Publish a host''s apps (e.g. Klatch) through more than one satellite simultaneously by running one outpost instance per extra satellite — WHEN a host must stay on its satellite and also serve members of another satellite (keywords: multi-satellite, second satellite, outpost-profile, up, down, instance, Klatch, share)'
tags:
    - outpost
    - satellite
    - klatch
    - multi-satellite
status: candidate
evidence: 'S565 2026-11-07: dev box and native Windows live runs; private two-satellite sandbox e2e PASS 16/16 twice; registration from the bashy src pin verified on rc.27; add no longer restarts the main outpost from outpost 80ad40f (bashy pin 789393c9)'
source:
    tool: claude
created: "2026-11-07T20:47:23Z"
updated: "2026-11-07T22:03:13Z"
---

## Goal

Keep one host's apps reachable through several satellites at once. The main outpost stays paired with its satellite; each extra satellite gets its own outpost instance on the same host, which proxies the same local apps.

## Steps

1. Get the command from Bashy's own release-pinned source (every installation can fetch it):

```sh
bashy src fetch bashy
bashy commands add outpost-profile \
  --set exec.0=bashy --set exec.1=--bashsharp \
  --set "exec.2=${BASHY_HOME:-$HOME/.bashy}/src/bashy/examples/outpost-profile/outpost-profile.bsh" \
  --set effects.0=read --set effects.1=write --set effects.2=exec
```

2. Pair with the extra satellite as a named profile (one-time pairing code from that satellite; use `--name` when this machine's host name is already taken there):

```sh
outpost-profile add <name> --code <code> --server <satellite-url> [--name <host>]
```

3. Run that profile as its own instance and publish apps through it. `--app` copies the app entry from the main outpost, including its SSO secret, so the app trusts identities from both satellites:

```sh
outpost-profile up <name> --app <app> [--app <app>]...
outpost-profile status
```

4. On the extra satellite, share the app with its members as usual.

5. Remove the instance with `outpost-profile down <name>`. The main outpost keeps serving, and the instance config is kept for the next `up`.

## What to know

- On releases up to rc.27, `outpost-profile add` briefly restarts the host's main outpost (outpost register applied the pairing to the running daemon); run it outside live use there. From the next release it no longer does.
- `up` needs the release that ships `up/down/status` (outpost-profile with `cmd_up`); older pins only have add/use/list. Check with `grep -c cmd_up` on the pinned script, or move the pin with `bashy src upgrade bashy`.
- Each instance has its own config, cache and service-unit store under `profiles/<name>/instance/` and its own loopback admin port (from 17778). LAN ssh, mesh, desktop, clipboard, shell, discovery, cluster, the ollama pool, shard and warm stay with the main instance.
- The instance runs as the Bashy service `outpost-<name>` (restart policy always), so it needs Bashy's service supervisor.
- The same email arriving from two satellites is one app account (Klatch SSO keys accounts by email).
- If the main app entry changes later (for example an SSO secret rotation), run `up <name> --app <app>` again; the instance keeps its copy until then.
- To adapt the command, shape it like any Bashy source: `bashy src branch bashy <topic>`, edit, run `go test ./examples/outpost-profile/`, and register that path.
- Basic outpost setup (install, pair, apps, built-ins) and the sharing model have their own pages: `set-up-outpost-install-pair-publish-apps-and-keep-a-host-private`, `satellite-sharing-model-one-owner-mutual-membership-pair-with-your-own-account`.
