---
id: 01a1b809-635a-725d-ab54-8b6cbd048f0d
seq: 11
form: page
type: runbook
title: Provision a satellite on DigitalOcean (bashy infra provision digitalocean; not verified live)
description: 'Documented-but-unverified path to stand up a satellite droplet with bashy infra provision digitalocean, then harden it (firewall, reserved IP) — WHEN a satellite must run on DigitalOcean instead of AWS; run every step as a dry run first (keywords: satellite, provision, digitalocean, droplet, infra provision, infra harden, firewall, reserved ip, enrollment)'
tags:
    - satellite
    - digitalocean
    - provision
    - infra
    - unverified
status: candidate
evidence: '2026-11-07: from bashy infra provision/harden/destroy --help and yoke pkg/fleet/infra_builtin.go (DO defaults r1/img are fixtures; token from DIGITALOCEAN_ACCESS_TOKEN) and pkg/provider/aws/firewall.go (rule set mirror); NOT run live'
source:
    tool: claude
created: "2026-11-07T22:03:05Z"
---

**Status: NOT verified live (2026-11-07).** Assembled from `bashy infra --help` and the yoke provider source; the AWS path is the verified one. Treat every step as a dry run first.

## Variables

```sh
SATELLITE_ID=<new satellite name>
DO_TOKEN_SECRET=<vault name of the DO token for the team that holds satellites>
OWNER_TOKEN_SECRET=<vault name of an owner bearer with the satellites scope>
DO_REGION=<region slug, e.g. sfo3>
DO_IMAGE=<image slug, e.g. ubuntu-24-04-x64>
```

## Steps

1. Check and register the satellite name at the master exactly as on AWS (steps 1-2 of the AWS satellite page).

2. Dry run, then live. Always pass `--region` and `--image`: the built-in DigitalOcean defaults (`r1`, `img`) are test-fixture values, not real slugs. `bashy infra` reads the token from `DIGITALOCEAN_ACCESS_TOKEN`, not from doctl's saved context:

```sh
bashy secret exec "$DO_TOKEN_SECRET=DIGITALOCEAN_ACCESS_TOKEN" -- \
  bashy infra provision digitalocean --satellite "$SATELLITE_ID" \
  --region "$DO_REGION" --size s-1vcpu-1gb --image "$DO_IMAGE" --dry-run
bashy secret exec "$DO_TOKEN_SECRET=DIGITALOCEAN_ACCESS_TOKEN" "$OWNER_TOKEN_SECRET=BASHY_FLEET_TOKEN" -- \
  bashy infra provision digitalocean --satellite "$SATELLITE_ID" \
  --region "$DO_REGION" --size s-1vcpu-1gb --image "$DO_IMAGE"
```

3. Harden after create. Unlike AWS, the DO adapter does not reserve an address first or apply a firewall during create; the first-boot script learns the address from DO metadata. Reconcile firewall membership and a reserved IP afterwards (preview with `--dry-run`):

```sh
DROPLET_ID=<id printed by the create>
bashy secret exec "$DO_TOKEN_SECRET=DIGITALOCEAN_ACCESS_TOKEN" -- \
  bashy infra harden --droplet "$DROPLET_ID" --firewall <satellite-firewall> --reserved-ip --dry-run
```

The satellite firewall should allow tcp 80, 443, 6443, 10250 and udp 8475-8476, with no internet-facing ssh (the AWS adapter applies the same rule set).

4. Sign-in and pairing codes: as on AWS (owner console via the master's Satellites page, Hosts, Generate invite code).

5. Tear down with `bashy infra destroy digitalocean --name "$DROPLET_ID" --yes`, or DigitalOcean's soft delete `bashy infra trash` / `bashy infra restore`. Never touch a droplet you did not create.

## Unverified points

- Whether a reserved IP attached after first boot changes the endpoint the satellite enrolled with.
- Whether `--dry-run` on digitalocean prints a budget verdict (the budget guard is documented for aws only).
